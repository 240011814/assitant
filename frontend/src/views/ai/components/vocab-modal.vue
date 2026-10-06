<script setup lang="ts">
import { ref } from "vue";
import { useMessage } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import { fetchAddVocabulary } from "@/service/api";

defineOptions({
  name: "VocabModal"
});

// 生词本弹窗: 从聊天消息一键添加词汇 (预填内容由父组件经 open()/setWord() 传入)
const props = defineProps<{
  /** 朗读单词 (复用父组件的语音实现, 保持同一 speechSynthesis 行为) */
  play: (text: string) => void;
}>();

const appStore = useAppStore();
const message = useMessage();

const show = ref(false);
const loading = ref(false);
const form = ref({
  word: "",
  phonetic: "",
  definition: "",
  example: "",
  confusingWords: "",
});

function resetForm() {
  form.value = {
    word: "",
    phonetic: "",
    definition: "",
    example: "",
    confusingWords: "",
  };
}

/** 打开弹窗; 传入预填表单 (如消息中的词汇建议) */
function open(preset?: Partial<typeof form.value>) {
  resetForm();
  if (preset) {
    form.value = { ...form.value, ...preset };
  }
  show.value = true;
}

/** 划词预填单词 (不打开弹窗, 与原行为一致) */
function setWord(word: string) {
  form.value.word = word;
}

async function submit() {
  if (!form.value.word.trim()) {
    message.warning("请输入单词");
    return;
  }

  loading.value = true;
  try {
    await fetchAddVocabulary(form.value);
    message.success("已添加到生词本");
    show.value = false;
  } catch (err: any) {
    message.error(`添加失败: ${err?.message || "未知错误"}`);
  } finally {
    loading.value = false;
  }
}

defineExpose({ open, setWord });
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="添加到生词本"
    :style="{ width: appStore.isMobile ? '95vw' : '' }"
    class="max-w-md"
    :segmented="{ content: 'soft' }"
  >
    <NForm
      :model="form"
      label-placement="left"
      :label-width="appStore.isMobile ? '60' : '80'"
    >
      <NFormItem label="单词" path="word">
        <div class="flex gap-2 w-full">
          <NInput
            v-model:value="form.word"
            placeholder="输入单词"
            class="flex-1"
          />
          <ButtonIcon
            icon="mdi:volume-high"
            class="text-20px text-primary"
            @click="props.play(form.word)"
          />
        </div>
      </NFormItem>
      <NFormItem label="音标" path="phonetic">
        <NInput v-model:value="form.phonetic" placeholder="输入音标 (可选)" />
      </NFormItem>
      <NFormItem label="释义" path="definition">
        <NInput
          v-model:value="form.definition"
          type="textarea"
          :autosize="{ minRows: 2 }"
          placeholder="输入中文释义"
        />
      </NFormItem>
      <NFormItem label="例句" path="example">
        <NInput
          v-model:value="form.example"
          type="textarea"
          :autosize="{ minRows: 2 }"
          placeholder="输入英文例句及翻译，如：I love coffee. (我爱咖啡。)"
        />
      </NFormItem>
      <NFormItem label="易混淆" path="confusingWords">
        <NInput
          v-model:value="form.confusingWords"
          type="textarea"
          :autosize="{ minRows: 2 }"
          placeholder="输入易混淆单词及翻译，如：Shook (摇动)"
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
