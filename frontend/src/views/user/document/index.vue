<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
  NButton,
  NCard,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NPagination,
  NPopconfirm,
  NSpin,
  NTag,
  NTooltip,
  NUpload,
  NUploadDragger,
  useMessage,
  type UploadCustomRequestOptions,
  type UploadFileInfo
} from 'naive-ui';
import { useAuth } from '@/hooks/business/auth';
import { useAppStore } from '@/store/modules/app';
import {
  fetchDocumentStatus,
  fetchDocuments,
  fetchDeleteDocument,
  fetchDocumentDownloadUrl,
  fetchDocumentText,
  fetchReindexDocument,
  fetchUploadDocument,
  type DocumentStatus,
  type UserDocumentItem
} from '@/service/api';

const message = useMessage();
const router = useRouter();
const { hasAuth } = useAuth();
const appStore = useAppStore();

const ACCEPT = '.pdf,.xlsx,.docx,.csv,.html,.htm,.md,.txt,.json';
const MAX_CONCURRENT = 5;
const FALLBACK_MAX_MB = 20;

// 功能状态: 存储未开启时展示引导页; 获取失败按已开启处理(回退旧展示)
const docStatus = ref<DocumentStatus | null>(null);
const statusLoading = ref(false);
async function loadStatus() {
  statusLoading.value = true;
  try {
    const { data } = await fetchDocumentStatus();
    if (data) docStatus.value = data;
  } finally {
    statusLoading.value = false;
  }
}
const featureEnabled = computed(() => !docStatus.value || docStatus.value.enabled);
const maxMB = computed(() => docStatus.value?.max_upload_mb || FALLBACK_MAX_MB);

const loading = ref(false);
const docs = ref<UserDocumentItem[]>([]);
const total = ref(0);
const page = reactive({ current: 1, size: 20 });

// 解析状态展示
const parseStatusMap: Record<string, { label: string; type: 'success' | 'error' | 'default' }> = {
  ok: { label: '已解析', type: 'success' },
  failed: { label: '解析失败', type: 'error' },
  none: { label: '不支持解析', type: 'default' }
};

// 向量索引状态展示 (RAG 语义检索)
const indexStatusMap: Record<string, { label: string; type: 'success' | 'error' | 'default' | 'info' | 'warning' }> = {
  ok: { label: '已索引', type: 'success' },
  failed: { label: '索引失败', type: 'error' },
  indexing: { label: '索引中', type: 'info' },
  pending: { label: '排队中', type: 'warning' },
  none: { label: '未索引', type: 'default' }
};

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

async function loadList() {
  loading.value = true;
  try {
    const { data, error } = await fetchDocuments(page.current, page.size);
    if (error || !data) {
      message.error(error?.message || '获取文档列表失败');
      return;
    }
    docs.value = data.items;
    total.value = data.total;
  } finally {
    loading.value = false;
  }
}

// ============ 上传 (拖拽区 + 受控文件列表, 逐文件状态展示) ============
const uploadFiles = ref<UploadFileInfo[]>([]);

// 多个文件同时完成时合并成一次列表刷新
let refreshTimer: ReturnType<typeof setTimeout> | null = null;
function refreshListSoon() {
  if (refreshTimer) clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => {
    refreshTimer = null;
    loadList();
  }, 600);
}

// 上传完成的记录 4 秒后自动清出列表, 避免长期堆积
let cleanTimer: ReturnType<typeof setTimeout> | null = null;
function scheduleCleanFinished() {
  if (cleanTimer) clearTimeout(cleanTimer);
  cleanTimer = setTimeout(() => {
    cleanTimer = null;
    uploadFiles.value = uploadFiles.value.filter(f => f.status === 'pending' || f.status === 'uploading');
  }, 4000);
}

function handleBeforeUpload({ file, fileList }: { file: UploadFileInfo; fileList: UploadFileInfo[] }) {
  const name = file.name || '';
  const ok = ACCEPT.split(',').some(ext => name.toLowerCase().endsWith(ext.slice(1)));
  if (!ok) {
    message.warning(`「${name}」类型不支持 (支持: pdf/xlsx/docx/csv/html/md/txt/json)`);
    return false;
  }
  if (file.file && file.file.size > maxMB.value * 1024 * 1024) {
    message.warning(`「${name}」超过大小上限 ${maxMB.value}MB`);
    return false;
  }
  const active = fileList.filter(f => f.status === 'pending' || f.status === 'uploading').length;
  if (active >= MAX_CONCURRENT) {
    message.warning(`最多同时上传 ${MAX_CONCURRENT} 个文件, 请等当前队列完成`);
    return false;
  }
  return true;
}

// 自定义上传: 交给后端 multipart 接口; onFinish/onError 驱动文件列表状态
async function handleCustomRequest({ file, onFinish, onError }: UploadCustomRequestOptions) {
  const raw = file.file;
  if (!raw) {
    onError();
    return;
  }
  try {
    const { data, error } = await fetchUploadDocument(raw);
    if (error || !data) {
      message.error(`「${file.name}」上传失败: ${error?.message || '未知错误'}`);
      onError();
      return;
    }
    message.success(data.parse_status === 'ok' ? `「${file.name}」上传并解析成功` : `「${file.name}」上传成功 (解析未完成, 详情见列表)`);
    onFinish();
    refreshListSoon();
  } finally {
    scheduleCleanFinished();
  }
}

async function handleDownload(doc: UserDocumentItem) {
  const { data, error } = await fetchDocumentDownloadUrl(doc.id);
  if (error || !data) {
    message.error(error?.message || '获取下载链接失败');
    return;
  }
  window.open(data.url, '_blank');
}

async function handleDelete(doc: UserDocumentItem) {
  const { error } = await fetchDeleteDocument(doc.id);
  if (error) {
    message.error(error?.message || '删除失败');
    return;
  }
  message.success('删除成功');
  await loadList();
}

// 建立/重建向量索引 (后台执行, 轮询列表看结果)
const reindexingId = ref<number | null>(null);
async function handleReindex(doc: UserDocumentItem) {
  reindexingId.value = doc.id;
  try {
    const { error } = await fetchReindexDocument(doc.id);
    if (error) {
      message.error(error?.message || '重建索引失败');
      return;
    }
    message.success(`「${doc.filename}」已加入索引队列`);
    await loadList();
  } finally {
    reindexingId.value = null;
  }
}

// 文本预览抽屉 (按字符区间分页拉取, 与 AI 读取同一套语义)
const showPreview = ref(false);
const previewDoc = ref<UserDocumentItem | null>(null);
const previewText = ref('');
const previewTotal = ref(0);
const previewLoading = ref(false);
const previewEnd = computed(() => previewText.value.length >= previewTotal.value);

async function openPreview(doc: UserDocumentItem) {
  if (doc.parse_status !== 'ok') {
    message.warning(doc.parse_status === 'failed' ? `解析失败: ${doc.parse_error}` : '该文件类型不支持解析为文本');
    return;
  }
  previewDoc.value = doc;
  previewText.value = '';
  previewTotal.value = 0;
  showPreview.value = true;
  await loadMorePreview(0);
}

async function loadMorePreview(offset: number) {
  if (!previewDoc.value) return;
  previewLoading.value = true;
  try {
    const { data, error } = await fetchDocumentText(previewDoc.value.id, offset, 6000);
    if (error || !data) {
      message.error(error?.message || '读取文本失败');
      return;
    }
    previewText.value += data.chunk;
    previewTotal.value = data.total_chars;
  } finally {
    previewLoading.value = false;
  }
}

function goConfig() {
  router.push('/system/config');
}

onMounted(() => {
  loadStatus();
  loadList();
});
</script>

<template>
  <div class="min-h-full p-2" :class="appStore.isMobile ? '' : 'p-4'">
    <NCard :bordered="false" size="small" title="文档管理">
      <template #header-extra>
        <div class="flex items-center gap-2">
          <NTag
            v-if="docStatus?.enabled"
            :type="docStatus.rag_ready ? 'success' : 'default'"
            size="small"
            :bordered="false"
            title="语义检索就绪后, AI 可用 search_user_documents 工具按语义检索文档内容"
          >
            {{ docStatus.rag_ready ? '语义检索已就绪' : '语义检索未开启' }}
          </NTag>
        </div>
      </template>

      <!-- 功能未开启: 引导去系统配置 -->
      <div v-if="docStatus && !docStatus.enabled" class="flex flex-col items-center gap-3 py-16">
        <SvgIcon icon="mdi:cloud-off-outline" class="text-56px text-gray-300 dark:text-gray-600" />
        <div class="text-base font-medium">文档功能未开启</div>
        <div class="text-sm text-gray-400 text-center max-w-100 leading-6">
          上传与管理文档依赖 S3 兼容对象存储, 当前未开启。
          请先在「系统配置 → 文档存储」中开启并填写 Endpoint / Bucket / 密钥等配置, 配置保存后立即生效。
        </div>
        <div class="flex items-center gap-2 mt-1">
          <NButton v-if="hasAuth('R_SUPER')" size="small" type="primary" @click="goConfig">
            <template #icon><SvgIcon icon="mdi:tune-variant" /></template>
            前往系统配置
          </NButton>
          <NButton size="small" quaternary :loading="statusLoading" @click="loadStatus">重新检测</NButton>
        </div>
      </div>

      <!-- 初始状态检测中 -->
      <div v-else-if="!docStatus && statusLoading" class="flex justify-center py-16">
        <NSpin size="medium" />
      </div>

      <template v-else>
        <!-- 拖拽上传区 -->
        <NUpload
          v-if="hasAuth('document:upload')"
          v-model:file-list="uploadFiles"
          :custom-request="handleCustomRequest"
          :before-upload="handleBeforeUpload"
          :accept="ACCEPT"
          multiple
        >
          <NUploadDragger>
            <div class="flex flex-col items-center gap-1 py-4">
              <SvgIcon icon="mdi:cloud-upload-outline" class="text-36px text-blue-400" />
              <div class="text-sm">点击或拖拽文件到此处上传</div>
              <div class="text-xs text-gray-400">
                支持 pdf / xlsx / docx / csv / html / md / txt / json, 单文件 ≤ {{ maxMB }}MB, 最多同时 {{ MAX_CONCURRENT }} 个;
                上传后 AI 可通过文档工具读取
              </div>
            </div>
          </NUploadDragger>
        </NUpload>

        <div class="mt-3 min-h-48">
          <div
            v-for="doc in docs"
            :key="doc.id"
            class="flex items-center gap-3 p-2 rounded border border-transparent hover:bg-gray-100 dark:hover:bg-gray-800 mb-1"
          >
            <SvgIcon icon="mdi:file-document-outline" class="text-20px text-blue-500 shrink-0" />
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium truncate">{{ doc.filename }}</div>
              <div class="text-xs text-gray-400 flex items-center gap-2 mt-0.5 flex-wrap">
                <span>{{ formatSize(doc.size_bytes) }}</span>
                <NTag :type="parseStatusMap[doc.parse_status]?.type || 'default'" size="tiny" :bordered="false">
                  {{ parseStatusMap[doc.parse_status]?.label || doc.parse_status }}
                </NTag>
                <span v-if="doc.parse_status === 'ok'">{{ doc.text_chars }} 字符</span>
                <NTooltip v-if="doc.parse_status === 'failed' && doc.parse_error">
                  <template #trigger>
                    <span class="text-red-400 cursor-help">失败原因</span>
                  </template>
                  {{ doc.parse_error }}
                </NTooltip>
                <NTag
                  v-if="doc.parse_status === 'ok'"
                  :type="indexStatusMap[doc.index_status]?.type || 'default'"
                  size="tiny"
                  :bordered="false"
                >
                  {{
                    doc.index_status === 'ok' && doc.chunk_count > 0
                      ? `已索引 · ${doc.chunk_count} 块`
                      : indexStatusMap[doc.index_status]?.label || doc.index_status
                  }}
                </NTag>
                <NTooltip v-if="doc.index_status === 'failed' && doc.index_error">
                  <template #trigger>
                    <span class="text-red-400 cursor-help">索引失败原因</span>
                  </template>
                  {{ doc.index_error }}
                </NTooltip>
                <span>{{ doc.created_at?.slice(0, 16).replace('T', ' ') }}</span>
              </div>
            </div>
            <div class="flex items-center gap-1 shrink-0">
              <NButton v-if="doc.parse_status === 'ok'" size="tiny" quaternary @click="openPreview(doc)">预览</NButton>
              <NButton
                v-if="doc.parse_status === 'ok' && hasAuth('document:upload')"
                size="tiny"
                quaternary
                :loading="reindexingId === doc.id"
                :disabled="doc.index_status === 'pending' || doc.index_status === 'indexing'"
                @click="handleReindex(doc)"
              >
                {{ doc.index_status === 'ok' ? '重建索引' : '建立索引' }}
              </NButton>
              <NButton size="tiny" quaternary @click="handleDownload(doc)">下载</NButton>
              <NPopconfirm v-if="hasAuth('document:delete')" @positive-click="handleDelete(doc)">
                <template #trigger>
                  <NButton size="tiny" quaternary type="error">删除</NButton>
                </template>
                确认删除「{{ doc.filename }}」？AI 将无法再读取该文档。
              </NPopconfirm>
            </div>
          </div>
          <NEmpty v-if="docs.length === 0 && !loading" description="暂无文档" class="py-10" />
        </div>

        <div class="flex justify-end mt-2">
          <NPagination
            v-model:page="page.current"
            v-model:page-size="page.size"
            :item-count="total"
            @update:page="loadList"
          />
        </div>
      </template>
    </NCard>

    <!-- 文本预览抽屉 -->
    <NDrawer v-model:show="showPreview" :width="appStore.isMobile ? '100%' : 560" placement="right">
      <NDrawerContent :title="previewDoc?.filename || '文档预览'" closable>
        <pre class="whitespace-pre-wrap text-13px leading-6 bg-gray-50 dark:bg-gray-800 rounded-lg p-3">{{ previewText }}</pre>
        <div class="flex justify-center mt-2">
          <NButton
            v-if="!previewEnd"
            size="small"
            secondary
            :loading="previewLoading"
            @click="loadMorePreview(previewText.length)"
          >
            继续加载 ({{ previewText.length }}/{{ previewTotal }})
          </NButton>
          <span v-else class="text-xs text-gray-400">已到结尾 (共 {{ previewTotal }} 字符)</span>
        </div>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
