<script lang="ts" setup>
import { ref } from 'vue';
import { addRecord } from '@/service/api';

const props = defineProps<{
  data: Api.Cut.RecordRequest | null;
}>();

interface Emits {
  (e: 'saved'): void;
}
const emit = defineEmits<Emits>();

const showModal = ref(false);
const inputName = ref('');
const saving = ref(false);

async function saveConfirm() {
  if (!props.data || saving.value) {
    return;
  }
  saving.value = true;
  try {
    // 合并输入的名称
    const payload = {
      ...props.data,
      name: inputName.value
    };
    const { data: res } = await addRecord(payload);
    if (res) {
      window.$message?.success('保存成功');
      showModal.value = false;
      inputName.value = '';
      emit('saved');
    } else {
      window.$message?.error('保存失败');
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div>
    <NButton type="primary" :disabled="!data" @click="showModal = true">保存</NButton>

    <NModal v-model:show="showModal" preset="dialog" title="输入名称">
      <div class="space-y-4">
        <NInput v-model:value="inputName" placeholder="请输入名称" />
      </div>
      <template #action>
        <NButton @click="showModal = false">取消</NButton>
        <NButton type="primary" :disabled="!inputName" :loading="saving" @click="saveConfirm">确定</NButton>
      </template>
    </NModal>
  </div>
</template>
