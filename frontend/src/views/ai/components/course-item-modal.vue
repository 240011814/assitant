<script setup lang="ts">
import { ref } from "vue";
import { useMessage } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import { fetchCourseList, fetchCreateCourseItem, type Course } from "@/service/api/course";

defineOptions({
  name: "CourseItemModal"
});

// 添加到课程包弹窗: 例句/翻译由父组件经 open() 预填, 课程包列表打开时加载
const appStore = useAppStore();
const message = useMessage();

const show = ref(false);
const loading = ref(false);
const options = ref<{ label: string; value: number }[]>([]);
const selectedCourseId = ref<number | null>(null);
const form = ref({
  english_sentence: "",
  chinese_translation: "",
});

async function loadCourseOptions() {
  try {
    const { data } = await fetchCourseList({ page_size: 100 });
    if (data?.list) {
      options.value = data.list.map((course: Course) => ({
        label: course.title,
        value: course.id,
      }));
    }
  } catch (err: any) {
    message.error(`加载课程包失败: ${err?.message || "未知错误"}`);
  }
}

/** 打开弹窗并预填例句 (同时加载课程包下拉; 不重置已选课程, 与原行为一致) */
function open(preset: { english_sentence: string; chinese_translation: string }) {
  form.value = { ...preset };
  loadCourseOptions();
  show.value = true;
}

async function submit() {
  if (!selectedCourseId.value) {
    message.warning("请选择课程包");
    return;
  }
  if (!form.value.english_sentence.trim()) {
    message.warning("请输入英文例句");
    return;
  }

  loading.value = true;
  try {
    await fetchCreateCourseItem(selectedCourseId.value, form.value);
    message.success("已添加到课程包");
    show.value = false;
  } catch (err: any) {
    message.error(`添加失败: ${err?.message || "未知错误"}`);
  } finally {
    loading.value = false;
  }
}

defineExpose({ open });
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="添加到课程包"
    :style="{ width: appStore.isMobile ? '95vw' : '' }"
    class="max-w-md"
    :segmented="{ content: 'soft' }"
  >
    <NForm
      :model="form"
      label-placement="left"
      :label-width="appStore.isMobile ? '60' : '80'"
    >
      <NFormItem label="课程包" path="course_id">
        <NSelect
          v-model:value="selectedCourseId"
          :options="options"
          placeholder="请选择课程包"
          filterable
        />
      </NFormItem>
      <NFormItem label="英文例句" path="english_sentence">
        <NInput
          v-model:value="form.english_sentence"
          type="textarea"
          :autosize="{ minRows: 3 }"
          placeholder="英文例句"
        />
      </NFormItem>
      <NFormItem label="中文翻译" path="chinese_translation">
        <NInput
          v-model:value="form.chinese_translation"
          type="textarea"
          :autosize="{ minRows: 2 }"
          placeholder="中文翻译（可选）"
        />
      </NFormItem>
    </NForm>
    <template #footer>
      <div class="flex justify-end gap-3">
        <NButton @click="show = false">取消</NButton>
        <NButton type="primary" :loading="loading" @click="submit">
          确认添加
        </NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped></style>
