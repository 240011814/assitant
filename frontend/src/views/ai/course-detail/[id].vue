<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NCard, NButton, NInput, NSpin, NEmpty, NSpace, NPopconfirm,
  NModal, NForm, NFormItem, NInputNumber, NSelect, useMessage
} from 'naive-ui'
import { fetchCourseDetail, fetchCreateCourseItem, fetchUpdateCourseItem, fetchDeleteCourseItem, fetchBatchCreateCourseItems, fetchBatchDeleteCourseItems, fetchUpdateCourse } from '@/service/api';
import type { CourseItem, CourseDetail } from '@/service/api';
import { stopSpeak, speak } from '@/utils/tts'

defineOptions({ name: 'AiCourseDetail' })

const route = useRoute()
const router = useRouter()
const message = useMessage()

const loading = ref(false)
const saving = ref(false)
const course = ref<CourseDetail | null>(null)
const courseId = computed(() => Number(route.params.id))

const editingCourse = ref(false)
const courseForm = ref({ title: '', description: '', tags: [] as string[], is_public: false })

const tagOptions = [
  { label: '四级', value: '四级' },
  { label: '六级', value: '六级' },
  { label: '考研', value: '考研' },
  { label: '雅思', value: '雅思' },
  { label: '托福', value: '托福' },
  { label: '小学', value: '小学' },
  { label: '初中', value: '初中' },
  { label: '高中', value: '高中' },
  { label: '日常对话', value: '日常对话' },
  { label: '商务英语', value: '商务英语' },
  { label: '旅游出行', value: '旅游出行' },
  { label: '学术英语', value: '学术英语' },
  { label: '面试求职', value: '面试求职' }
]

const showAddItem = ref(false)
const itemForm = ref({ english_sentence: '', chinese_translation: '', sort_order: 0 })

const showBatchImport = ref(false)
const batchText = ref('')
const batchPreview = ref<{ english_sentence: string; chinese_translation: string }[]>([])

const showEditItem = ref(false)
const editingItemId = ref<number | null>(null)
const editItemForm = ref({ english_sentence: '', chinese_translation: '', sort_order: 0 })

const searchKeyword = ref('')
const playingItemId = ref<number | null>(null)
const selectedIds = ref<number[]>([])
const isSelecting = ref(false)

const filteredItems = computed(() => {
  if (!course.value?.items) return []
  if (!searchKeyword.value.trim()) return course.value.items
  const keyword = searchKeyword.value.toLowerCase()
  return course.value.items.filter(item =>
    item.english_sentence.toLowerCase().includes(keyword) ||
    item.chinese_translation.toLowerCase().includes(keyword)
  )
})

const loadCourse = async () => {
  loading.value = true
  try {
    const { data } = await fetchCourseDetail(courseId.value)
    if (data) {
      if (!data.items) data.items = []
      course.value = data
      courseForm.value = { title: data.title, description: data.description, tags: data.tags ? data.tags.split(',') : [], is_public: data.is_public }
    }
  } catch {
    message.error('加载课程失败')
  } finally {
    loading.value = false
  }
}

const handleUpdateCourse = async () => {
  if (!courseForm.value.title.trim()) return message.warning('请输入课程标题')
  saving.value = true
  try {
    await fetchUpdateCourse(courseId.value, courseForm.value)
    message.success('更新成功')
    editingCourse.value = false
    loadCourse()
  } catch {
    message.error('更新失败')
  } finally {
    saving.value = false
  }
}

const handleAddItem = async () => {
  if (!itemForm.value.english_sentence.trim()) return message.warning('请输入英语句子')
  saving.value = true
  try {
    await fetchCreateCourseItem(courseId.value, itemForm.value)
    message.success('添加成功')
    showAddItem.value = false
    itemForm.value = { english_sentence: '', chinese_translation: '', sort_order: 0 }
    loadCourse()
  } catch {
    // 错误消息已由 request 函数自动显示
  } finally {
    saving.value = false
  }
}

const handleEditItem = (item: CourseItem) => {
  editingItemId.value = item.id
  editItemForm.value = { english_sentence: item.english_sentence, chinese_translation: item.chinese_translation, sort_order: item.sort_order }
  showEditItem.value = true
}

const handleUpdateItem = async () => {
  if (!editingItemId.value || !editItemForm.value.english_sentence.trim()) return message.warning('请输入英语句子')
  saving.value = true
  try {
    await fetchUpdateCourseItem(courseId.value, editingItemId.value, editItemForm.value)
    message.success('更新成功')
    showEditItem.value = false
    editingItemId.value = null
    loadCourse()
  } catch {
    message.error('更新失败')
  } finally {
    saving.value = false
  }
}

const handleDeleteItem = async (itemId: number) => {
  try {
    await fetchDeleteCourseItem(courseId.value, itemId)
    message.success('删除成功')
    loadCourse()
  } catch {
    message.error('删除失败')
  }
}

const toggleSelect = (itemId: number) => {
  const index = selectedIds.value.indexOf(itemId)
  if (index === -1) {
    selectedIds.value.push(itemId)
  } else {
    selectedIds.value.splice(index, 1)
  }
}

const toggleSelectAll = () => {
  if (selectedIds.value.length === filteredItems.value.length) {
    selectedIds.value = []
  } else {
    selectedIds.value = filteredItems.value.map(item => item.id)
  }
}

const handleBatchDelete = async () => {
  if (!selectedIds.value.length) return message.warning('请选择要删除的句子')
  try {
    await fetchBatchDeleteCourseItems(courseId.value, selectedIds.value)
    message.success(`成功删除 ${selectedIds.value.length} 条`)
    selectedIds.value = []
    isSelecting.value = false
    loadCourse()
  } catch {
    message.error('批量删除失败')
  }
}

const cancelSelect = () => {
  isSelecting.value = false
  selectedIds.value = []
}

const parseBatchText = () => {
  batchPreview.value = batchText.value.trim().split('\n').filter(l => l.trim()).map(line => {
    if (line.includes('|')) {
      const [en, cn] = line.split('|')
      return { english_sentence: en.trim(), chinese_translation: cn?.trim() || '' }
    }
    const m = line.match(/^(.*?)（(.*?)）$/) || line.match(/^(.*?)\((.*?)\)$/)
    if (m) return { english_sentence: m[1].trim(), chinese_translation: m[2].trim() }
    return { english_sentence: line.trim(), chinese_translation: '' }
  }).filter(i => i.english_sentence)
}

const handleBatchImport = async () => {
  if (!batchPreview.value.length) return message.warning('没有可导入的内容')
  saving.value = true
  try {
    const { data } = await fetchBatchCreateCourseItems(courseId.value, batchPreview.value.map((item, i) => ({ ...item, sort_order: i })))
    if (data) {
      const importedCount = data.items ? data.items.length : 0
      const duplicateCount = data.duplicate_count || 0
      if (duplicateCount > 0) {
        message.success(`成功导入 ${importedCount} 条，跳过 ${duplicateCount} 条重复`)
      } else {
        message.success(`成功导入 ${importedCount} 条`)
      }
    }
    showBatchImport.value = false
    batchText.value = ''
    batchPreview.value = []
    loadCourse()
  } catch {
    message.error('导入失败')
  } finally {
    saving.value = false
  }
}

const handlePlay = async (item: CourseItem) => {
  if (playingItemId.value === item.id) {
    stopSpeak()
    playingItemId.value = null
    return
  }
  playingItemId.value = item.id
  try {
    await speak(item.english_sentence, { lang: 'en-US', rate: 0.9 })
  } catch {
    message.error('语音播放失败')
  } finally {
    playingItemId.value = null
  }
}

onMounted(loadCourse)
</script>

<template>
  <div class="h-full flex flex-col p-4 gap-4">
    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <NSpin size="large" />
    </div>

    <template v-else-if="course">
      <div class="flex items-center gap-3">
        <NButton quaternary circle @click="router.push({ name: 'ai_course' })">
          <template #icon>
            <SvgIcon icon="mdi:arrow-left" class="text-lg" />
          </template>
        </NButton>
        <h2 class="text-lg font-semibold m-0">{{ course.title }}</h2>
      </div>

      <div class="flex justify-between items-center">
        <div class="flex items-center gap-4">
          <h2 class="text-lg font-semibold m-0">课程条目</h2>
          <NInput
            v-model:value="searchKeyword"
            placeholder="搜索句子..."
            clearable
            style="width: 250px"
          >
            <template #prefix>
              <SvgIcon icon="mdi:magnify" />
            </template>
          </NInput>
        </div>
        <NSpace>
          <template v-if="isSelecting">
            <NButton @click="toggleSelectAll">
              {{ selectedIds.length === filteredItems.length ? "取消全选" : "全选" }}
            </NButton>
            <NButton
              type="error"
              :disabled="!selectedIds.length"
              @click="handleBatchDelete"
            >
              <template #icon>
                <SvgIcon icon="mdi:delete" />
              </template>
              删除 ({{ selectedIds.length }})
            </NButton>
            <NButton @click="cancelSelect">取消</NButton>
          </template>
          <template v-else>
            <NButton :disabled="!filteredItems.length" @click="isSelecting = true">
              <template #icon>
                <SvgIcon icon="mdi:checkbox-multiple-marked-outline" />
              </template>
              批量删除
            </NButton>
            <NButton @click="showBatchImport = true">
              <template #icon>
                <SvgIcon icon="mdi:upload" />
              </template>
              批量导入
            </NButton>
            <NButton type="primary" @click="showAddItem = true">
              <template #icon>
                <SvgIcon icon="mdi:plus" />
              </template>
              添加句子
            </NButton>
          </template>
        </NSpace>
      </div>

      <div v-if="!filteredItems.length" class="flex-1 flex items-center justify-center">
        <NEmpty
          :description="searchKeyword ? '没有匹配的句子' : '暂无句子，点击上方按钮添加'"
        />
      </div>

      <div v-else class="flex-1 overflow-auto">
        <div class="grid gap-3">
          <NCard
            v-for="(item, index) in filteredItems"
            :key="item.id"
            class="item-card hover:shadow-md transition-shadow"
            :class="{ 'ring-2 ring-primary': selectedIds.includes(item.id) }"
            size="small"
            @click="isSelecting ? toggleSelect(item.id) : undefined"
          >
            <div class="flex items-start justify-between gap-4">
              <div class="flex-1">
                <div class="flex items-start gap-3">
                  <div v-if="isSelecting" class="flex items-center">
                    <SvgIcon
                      :icon="
                        selectedIds.includes(item.id)
                          ? 'mdi:checkbox-marked-circle'
                          : 'mdi:checkbox-blank-circle-outline'
                      "
                      :class="
                        selectedIds.includes(item.id) ? 'text-primary' : 'text-gray-400'
                      "
                      class="text-xl cursor-pointer"
                    />
                  </div>
                  <span class="text-gray-400 text-sm font-mono min-w-[24px]">{{
                    index + 1
                  }}</span>
                  <div class="flex-1">
                    <div class="font-medium text-base mb-1">
                      {{ item.english_sentence }}
                    </div>
                    <div class="text-gray-500 text-sm">
                      {{ item.chinese_translation || "暂无翻译" }}
                    </div>
                  </div>
                </div>
              </div>
              <NSpace v-if="!isSelecting" size="small">
                <ButtonIcon
                  :icon="
                    playingItemId === item.id ? 'mdi:volume-high' : 'mdi:volume-medium'
                  "
                  :tooltip-content="playingItemId === item.id ? '停止播放' : '播放发音'"
                  :class="playingItemId === item.id ? 'text-primary' : ''"
                  @click.stop="handlePlay(item)"
                />
                <ButtonIcon
                  icon="mdi:pencil-outline"
                  tooltip-content="编辑"
                  @click.stop="handleEditItem(item)"
                />
                <NPopconfirm
                  placement="top"
                  @positive-click.stop="handleDeleteItem(item.id)"
                >
                  <template #trigger>
                    <div @click.stop>
                      <ButtonIcon
                        icon="mdi:trash-can-outline"
                        tooltip-content="删除"
                        class="text-error"
                      />
                    </div>
                  </template>
                  确定删除此句子？
                </NPopconfirm>
              </NSpace>
            </div>
          </NCard>
        </div>
      </div>
    </template>

    <NModal
      v-model:show="editingCourse"
      title="编辑课程信息"
      preset="dialog"
      positive-text="保存"
      negative-text="取消"
      :loading="saving"
      @positive-click="handleUpdateCourse"
    >
      <NForm :model="courseForm" label-placement="top">
        <NFormItem label="课程标题" required>
          <NInput v-model:value="courseForm.title" placeholder="请输入课程标题" />
        </NFormItem>
        <NFormItem label="课程标签">
          <NSelect
            v-model:value="courseForm.tags"
            :options="tagOptions"
            multiple
            placeholder="请选择标签"
          />
        </NFormItem>
        <NFormItem label="课程描述">
          <NInput
            v-model:value="courseForm.description"
            type="textarea"
            placeholder="请输入课程描述"
            :rows="3"
          />
        </NFormItem>
        <NFormItem label="公开课程">
          <NSwitch v-model:value="courseForm.is_public" />
          <span class="ml-2 text-sm text-gray-500">开启后所有用户可见</span>
        </NFormItem>
      </NForm>
    </NModal>

    <NModal
      v-model:show="showAddItem"
      title="添加句子"
      preset="dialog"
      positive-text="保存"
      negative-text="取消"
      :loading="saving"
      @positive-click="handleAddItem"
    >
      <NForm :model="itemForm" label-placement="top">
        <NFormItem label="英语句子" required>
          <NInput
            v-model:value="itemForm.english_sentence"
            type="textarea"
            placeholder="请输入英语句子"
            :rows="2"
          />
        </NFormItem>
        <NFormItem label="中文翻译">
          <NInput
            v-model:value="itemForm.chinese_translation"
            type="textarea"
            placeholder="请输入中文翻译"
            :rows="2"
          />
        </NFormItem>
        <NFormItem label="排序">
          <NInputNumber
            v-model:value="itemForm.sort_order"
            :min="0"
            placeholder="排序号"
          />
        </NFormItem>
      </NForm>
    </NModal>

    <NModal
      v-model:show="showEditItem"
      title="编辑句子"
      preset="dialog"
      positive-text="保存"
      negative-text="取消"
      :loading="saving"
      @positive-click="handleUpdateItem"
    >
      <NForm :model="editItemForm" label-placement="top">
        <NFormItem label="英语句子" required>
          <NInput
            v-model:value="editItemForm.english_sentence"
            type="textarea"
            placeholder="请输入英语句子"
            :rows="2"
          />
        </NFormItem>
        <NFormItem label="中文翻译">
          <NInput
            v-model:value="editItemForm.chinese_translation"
            type="textarea"
            placeholder="请输入中文翻译"
            :rows="2"
          />
        </NFormItem>
        <NFormItem label="排序">
          <NInputNumber
            v-model:value="editItemForm.sort_order"
            :min="0"
            placeholder="排序号"
          />
        </NFormItem>
      </NForm>
    </NModal>

    <NModal
      v-model:show="showBatchImport"
      title="批量导入"
      preset="dialog"
      positive-text="导入"
      negative-text="取消"
      style="width: 700px"
      :loading="saving"
      @positive-click="handleBatchImport"
    >
      <div class="space-y-4">
        <div>
          <div class="mb-2 font-medium">输入内容（每行一句）</div>
          <div class="mb-2 text-sm text-gray-500">
            支持格式：English | 中文 或 English（中文） 或 English(中文)
          </div>
          <NInput
            v-model:value="batchText"
            type="textarea"
            placeholder="Hello, how are you? | 你好&#10;Thank you.（谢谢）"
            :rows="10"
            @update:value="parseBatchText"
          />
        </div>
        <div v-if="batchPreview.length">
          <div class="mb-2 font-medium">预览 ({{ batchPreview.length }}条)</div>
          <div class="max-h-60 overflow-auto border rounded-lg p-3 bg-gray-50">
            <div
              v-for="(item, i) in batchPreview"
              :key="i"
              class="py-2 border-b last:border-0"
            >
              <div class="font-medium">{{ item.english_sentence }}</div>
              <div class="text-sm text-gray-500">{{ item.chinese_translation }}</div>
            </div>
          </div>
        </div>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.item-card {
  border-radius: 8px;
  transition: all 0.2s ease;
}

.item-card:hover {
  transform: translateY(-1px);
}

:deep(.n-card-header) {
  padding: 12px 16px;
}

:deep(.n-card__content) {
  padding: 12px 16px;
}
</style>
