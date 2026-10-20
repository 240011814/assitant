<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { addWatchlist, getWatchlistGroups } from '@/service/api'

defineOptions({ name: 'WatchlistAddDialog' })

interface Props {
  show: boolean
  code: string
  name?: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'added'): void
}>()

const message = useMessage()

const groupOptions = ref<string[]>([])
const groupName = ref<string | null>(null)
const note = ref('')
const saving = ref(false)

const showModel = computed({
  get: () => props.show,
  set: value => emit('update:show', value)
})

const selectOptions = computed(() => groupOptions.value.map(g => ({ label: g, value: g })))

watch(
  () => props.show,
  async value => {
    if (!value) return
    groupName.value = null
    note.value = ''
    try {
      const { data } = await getWatchlistGroups()
      groupOptions.value = data || []
    } catch {
      groupOptions.value = []
    }
  }
)

async function handleConfirm() {
  saving.value = true
  try {
    await addWatchlist({
      code: props.code,
      groupName: groupName.value || undefined,
      note: note.value || undefined
    })
    message.success('已添加到自选股')
    emit('added')
    showModel.value = false
  } catch (e: any) {
    message.error(e.message || '添加失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NModal v-model:show="showModel" preset="card" title="添加到自选" style="width: 420px" class="max-w-[96vw]">
    <div class="space-y-3">
      <div>
        <label class="block text-sm text-gray-600 mb-1">股票</label>
        <NInput :value="name ? `${code} ${name}` : code" disabled />
      </div>
      <div>
        <label class="block text-sm text-gray-600 mb-1">分组</label>
        <NSelect
          v-model:value="groupName"
          :options="selectOptions"
          placeholder="选择或输入新分组"
          filterable
          tag
          clearable
        />
      </div>
      <div>
        <label class="block text-sm text-gray-600 mb-1">备注</label>
        <NInput v-model:value="note" placeholder="备注(可选)" />
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="showModel = false">取消</NButton>
        <NButton type="primary" :loading="saving" @click="handleConfirm">确定</NButton>
      </div>
    </template>
  </NModal>
</template>