<script setup lang="ts">
import { ref, onActivated } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, NCard, NButton, NTag, NEmpty, NSpin, NModal, NForm, NFormItem, NInput, NSwitch, NSpace, NPopconfirm, NPagination, NSelect } from 'naive-ui'
import { fetchCourseList, fetchCreateCourse, fetchUpdateCourse, fetchDeleteCourse, fetchTrainingStatus, fetchUpdateTrainingStatus, type Course, type UserCourseTraining } from '@/service/api'
import { useAuth } from '@/hooks/business/auth'

defineOptions({ name: 'AiCourse' })

const router = useRouter()
const message = useMessage()
const { hasAuth } = useAuth()

const loading = ref(false)
const courses = ref<Course[]>([])
const showModal = ref(false)
const submitting = ref(false)
const trainingRecords = ref<Record<number, UserCourseTraining>>({})

const editingCourseId = ref<number | null>(null)
const showEditModal = ref(false)
const editForm = ref({
  title: '',
  description: '',
  tags: [] as string[],
  is_public: false
})

const keyword = ref('')
const isPublicFilter = ref<boolean | null>(null)
const tagFilter = ref('')
const currentPage = ref(1)
const pageSize = ref(12)
const total = ref(0)

const formData = ref({
  title: '',
  description: '',
  tags: [] as string[],
  is_public: false
})

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



const filterOptions = [
  { label: '全部', value: 'all' },
  { label: '公开', value: 'public' },
  { label: '私有', value: 'private' }
]

const publicFilterValue = ref<string>('all')

const loadTrainingRecords = async () => {
  // 并发请求各课程的训练状态, 避免逐个 await 串行等待
  await Promise.all(
    courses.value.map(async course => {
      const { data } = await fetchTrainingStatus(course.id)
      if (data) {
        trainingRecords.value[course.id] = data
      }
    })
  )
}

const loadCourses = async () => {
  loading.value = true
  try {
    const { data } = await fetchCourseList({
      keyword: keyword.value || undefined,
      is_public: isPublicFilter.value ?? undefined,
      tag: tagFilter.value || undefined,
      page: currentPage.value,
      page_size: pageSize.value
    })
    if (data) {
      courses.value = data.list
      total.value = data.total
      loadTrainingRecords()
    }
  } catch {
    message.error('加载课程包失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  currentPage.value = 1
  loadCourses()
}

const handleFilterChange = (value: string) => {
  publicFilterValue.value = value
  if (value === 'all') {
    isPublicFilter.value = null
  } else if (value === 'public') {
    isPublicFilter.value = true
  } else {
    isPublicFilter.value = false
  }
  currentPage.value = 1
  loadCourses()
}

const handlePageChange = (page: number) => {
  currentPage.value = page
  loadCourses()
}

const handleCreate = () => {
  formData.value = { title: '', description: '', tags: [], is_public: false }
  showModal.value = true
}

const handleSubmit = async () => {
  if (!formData.value.title.trim()) {
    message.warning('请输入课程标题')
    return
  }
  submitting.value = true
  try {
    const { data, error } = await fetchCreateCourse(formData.value)
    // 失败(data 为空/error 存在)时 return, 不关闭弹窗
    if (error || !data) {
      message.error('创建失败')
      return
    }
    message.success('创建成功')
    showModal.value = false
    loadCourses()
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (id: number) => {
  try {
    await fetchDeleteCourse(id)
    message.success('删除成功')
    loadCourses()
  } catch {
    message.error('删除失败')
  }
}

const handleEdit = (course: Course) => {
  editingCourseId.value = course.id
  editForm.value = {
    title: course.title,
    description: course.description,
    tags: course.tags ? course.tags.split(',') : [],
    is_public: course.is_public
  }
  showEditModal.value = true
}

const handleUpdateSubmit = async () => {
  if (!editForm.value.title.trim()) {
    message.warning('请输入课程标题')
    return
  }
  if (!editingCourseId.value) return
  submitting.value = true
  try {
    await fetchUpdateCourse(editingCourseId.value, editForm.value)
    message.success('更新成功')
    showEditModal.value = false
    loadCourses()
  } catch {
    message.error('更新失败')
  } finally {
    submitting.value = false
  }
}

const goToPractice = async (id: number) => {
  try {
    const { data } = await fetchUpdateTrainingStatus(id, 'in_progress')
    if (data) {
      trainingRecords.value[id] = data
    }
  } catch {
    // 忽略错误，继续跳转
  }
  router.push({ name: 'ai_exercise', query: { courseId: id } })
}



const goToEdit = (id: number) => {
  router.push({ name: 'ai_course-detail', params: { id } })
}

// KeepAlive 缓存下 onActivated 激活时加载, 避免与 onMounted 双触发
onActivated(() => {
  loadCourses()
})
</script>

<template>
  <div class="h-full flex flex-col p-4 gap-4">
    <div class="flex items-center justify-between">
      <h2 class="text-2xl font-bold">课程包管理</h2>
      <NButton v-if="hasAuth('ai:course:create')" type="primary" @click="handleCreate">
        创建课程包
      </NButton>
    </div>

    <div class="flex items-center gap-4">
      <NInput
        v-model:value="keyword"
        placeholder="搜索课程包..."
        clearable
        style="width: 250px"
        @keyup.enter="handleSearch"
        @clear="handleSearch"
      />
      <NSelect
        :value="tagFilter"
        :options="tagOptions"
        placeholder="筛选标签"
        clearable
        style="width: 140px"
        @update:value="(val: string) => { tagFilter = val || ''; currentPage = 1; loadCourses() }"
      />
      <NSelect
        :value="publicFilterValue"
        :options="filterOptions"
        placeholder="筛选状态"
        style="width: 120px"
        @update:value="handleFilterChange"
      />
      <NButton type="primary" @click="handleSearch">搜索</NButton>
    </div>

    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <NSpin size="large" />
    </div>

    <div v-else-if="courses.length === 0" class="flex-1 flex items-center justify-center">
      <NEmpty description="暂无课程包，点击上方按钮创建" />
    </div>

    <template v-else>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        <NCard
          v-for="course in courses"
          :key="course.id"
          class="cursor-pointer hover:shadow-lg transition-all duration-300 hover:-translate-y-1"
          @click="goToEdit(course.id)"
        >
          <template #header>
            <div class="flex items-center justify-between">
              <span class="text-lg font-semibold truncate">{{ course.title }}</span>
              <div class="flex items-center gap-1">
                <template v-if="course.tags">
                  <NTag
                    v-for="tag in course.tags.split(',')"
                    :key="tag"
                    type="info"
                    size="small"
                  >
                    {{ tag }}
                  </NTag>
                </template>
                <NTag v-if="course.is_public" type="success" size="small">公开</NTag>
                <NTag v-else type="default" size="small">私有</NTag>
              </div>
            </div>
          </template>

          <div class="text-gray-500 mb-4 line-clamp-2 h-10">
            {{ course.description || "暂无描述" }}
          </div>

          <div class="flex items-center justify-between text-sm text-gray-400 mb-2">
            <span class="flex items-center gap-1">
              <span class="i-carbon-document text-base"></span>
              {{ course.item_count }} 个句子
            </span>
            <span v-if="trainingRecords[course.id]" class="flex items-center gap-1">
              <NTag
                :type="
                  trainingRecords[course.id].training_status === 'completed'
                    ? 'success'
                    : trainingRecords[course.id].training_status === 'in_progress'
                      ? 'warning'
                      : 'default'
                "
                size="small"
              >
                {{
                  trainingRecords[course.id].training_status === "completed"
                    ? "已完成"
                    : trainingRecords[course.id].training_status === "in_progress"
                      ? "进行中"
                      : "未开始"
                }}
              </NTag>
              <span>{{ trainingRecords[course.id].training_count }} 次</span>
            </span>
          </div>

          <template #action>
            <NSpace justify="end">
              <NButton size="small" @click.stop="handleEdit(course)"> 编辑 </NButton>

              <NButton
                size="small"
                type="primary"
                :disabled="course.item_count === 0"
                @click.stop="goToPractice(course.id)"
              >
                开始练习
              </NButton>
              <NPopconfirm @positive-click.stop="handleDelete(course.id)">
                <template #trigger>
                  <NButton
                    v-if="hasAuth('ai:course:delete')"
                    size="small"
                    type="error"
                    @click.stop
                  >
                    删除
                  </NButton>
                </template>
                确定删除此课程包吗？
              </NPopconfirm>
            </NSpace>
          </template>
        </NCard>
      </div>

      <div class="flex justify-center mt-4">
        <NPagination
          v-model:page="currentPage"
          :page-size="pageSize"
          :item-count="total"
          :page-slot="7"
          @update:page="handlePageChange"
        />
      </div>
    </template>

    <NModal
      v-model:show="showModal"
      title="创建课程包"
      preset="dialog"
      positive-text="保存"
      negative-text="取消"
      :loading="submitting"
      @positive-click="handleSubmit"
    >
      <NForm :model="formData" label-placement="top">
        <NFormItem label="课程标题" required>
          <NInput
            v-model:value="formData.title" placeholder="请输入课程标题"
          />
        </NFormItem>
        <NFormItem label="课程标签">
          <NSelect
            v-model:value="formData.tags"
            :options="tagOptions"
            multiple
            placeholder="请选择标签"
          />
        </NFormItem>
        <NFormItem label="课程描述">
          <NInput
            v-model:value="formData.description"
            type="textarea"
            placeholder="请输入课程描述"
            :rows="3"
          />
        </NFormItem>
        <NFormItem label="公开课程">
          <NSwitch v-model:value="formData.is_public" />
          <span class="ml-2 text-sm text-gray-500">开启后所有用户可见</span>
        </NFormItem>
      </NForm>
    </NModal>

    <NModal
      v-model:show="showEditModal"
      title="编辑课程包"
      preset="dialog"
      positive-text="保存"
      negative-text="取消"
      :loading="submitting"
      @positive-click="handleUpdateSubmit"
    >
      <NForm :model="editForm" label-placement="top">
        <NFormItem label="课程标题" required>
          <NInput
            v-model:value="editForm.title" placeholder="请输入课程标题"
          />
        </NFormItem>
        <NFormItem label="课程标签">
          <NSelect
            v-model:value="editForm.tags"
            :options="tagOptions"
            multiple
            placeholder="请选择标签"
          />
        </NFormItem>
        <NFormItem label="课程描述">
          <NInput
            v-model:value="editForm.description"
            type="textarea"
            placeholder="请输入课程描述"
            :rows="3"
          />
        </NFormItem>
        <NFormItem label="公开课程">
          <NSwitch v-model:value="editForm.is_public" />
          <span class="ml-2 text-sm text-gray-500">开启后所有用户可见</span>
        </NFormItem>
      </NForm>
    </NModal>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

:deep(.n-card) {
  border-radius: 12px;
  overflow: hidden;
}

:deep(.n-card:hover) {
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

:deep(.n-card-header) {
  padding: 16px 20px;
  border-bottom: 1px solid #f0f0f0;
}

:deep(.n-card__content) {
  padding: 16px 20px;
}

:deep(.n-card__action) {
  padding: 12px 20px;
  border-top: 1px solid #f0f0f0;
}
</style>
