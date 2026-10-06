<script setup lang="ts">
import { ref } from "vue";
import { useMessage } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import { fetchAddNote } from "@/service/api";

defineOptions({
  name: "NoteModal"
});

// 添加笔记弹窗: 预填 (标题/分类/内容) 由父组件经 open() 传入,
// 正文右侧的 markdown 实时预览复用父组件的 renderMarkdown
const props = defineProps<{
  renderMarkdown: (content: string) => string;
}>();

const appStore = useAppStore();
const message = useMessage();

const show = ref(false);
const loading = ref(false);
const form = ref({
  title: "",
  category: "",
  content: "",
});

/** 打开弹窗并预填 (标题截取/分类推断等预处理在父组件完成) */
function open(preset: { title: string; category: string; content: string }) {
  form.value = { ...preset };
  show.value = true;
}

async function submit() {
  if (!form.value.title.trim()) {
    message.warning("请输入标题");
    return;
  }
  if (!form.value.category.trim()) {
    message.warning("请输入分类");
    return;
  }
  if (!form.value.content.trim()) {
    message.warning("请输入内容");
    return;
  }

  loading.value = true;
  try {
    await fetchAddNote(form.value);
    message.success("笔记添加成功");
    show.value = false;
  } catch (err: any) {
    message.error(`添加失败: ${err?.message || "未知错误 "}`);
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
    title="添加笔记"
    :style="{ width: appStore.isMobile ? '95vw' : '800px' }"
    :segmented="{ content: 'soft' }"
  >
    <NForm
      :model="form"
      label-placement="left"
      :label-width="appStore.isMobile ? '60' : '80'"
    >
      <div :class="appStore.isMobile ? 'flex flex-col gap-2' : 'flex gap-4'">
        <NFormItem label="标题" path="title" class="flex-1">
          <NInput v-model:value="form.title" placeholder="输入笔记标题" />
        </NFormItem>
        <NFormItem
          label="分类"
          path="category"
          :style="appStore.isMobile ? {} : { width: '240px' }"
        >
          <NInput v-model:value="form.category" placeholder="输入笔记分类" />
        </NFormItem>
      </div>
      <NFormItem label="内容" path="content">
        <div
          :class="
            appStore.isMobile
              ? 'flex flex-col gap-4 w-full'
              : 'grid grid-cols-2 gap-4 w-full'
          "
        >
          <NInput
            v-model:value="form.content"
            type="textarea"
            :autosize="
              appStore.isMobile
                ? { minRows: 6, maxRows: 10 }
                : { minRows: 12, maxRows: 15 }
            "
            placeholder="输入笔记内容"
          />
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div
            class="prose dark:prose-invert max-w-none overflow-y-auto p-4 border border-gray-200 dark:border-gray-700 rounded-md bg-gray-50/50 dark:bg-dark-100 text-sm leading-relaxed"
            style="height: 100%; max-height: 350px"
            v-html="props.renderMarkdown(form.content)"
          ></div>
        </div>
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
