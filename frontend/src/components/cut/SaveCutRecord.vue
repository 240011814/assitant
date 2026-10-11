<script lang="ts" setup>
import { ref } from 'vue';
import { $t } from '@/locales';
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
      window.$message?.success($t('page.cut.saveSuccess'));
      showModal.value = false;
      inputName.value = '';
      emit('saved');
    } else {
      window.$message?.error($t('page.cut.saveFailed'));
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div>
    <NButton type="primary" :disabled="!data" @click="showModal = true">{{ $t('common.save') }}</NButton>

    <NModal v-model:show="showModal" preset="dialog" :title="$t('page.cut.inputNameTitle')">
      <div class="space-y-4">
        <NInput v-model:value="inputName" :placeholder="$t('page.cut.inputName')" />
      </div>
      <template #action>
        <NButton @click="showModal = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :disabled="!inputName" :loading="saving" @click="saveConfirm">{{ $t('common.confirm') }}</NButton>
      </template>
    </NModal>
  </div>
</template>
