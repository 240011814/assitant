<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  NButton,
  NCard,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NPagination,
  NPopconfirm,
  NTag,
  NTooltip,
  NUpload,
  useMessage,
  type UploadCustomRequestOptions,
  type UploadFileInfo
} from 'naive-ui';
import { useAuth } from '@/hooks/business/auth';
import { useAppStore } from '@/store/modules/app';
import {
  fetchDocuments,
  fetchDeleteDocument,
  fetchDocumentDownloadUrl,
  fetchDocumentText,
  fetchUploadDocument,
  type UserDocumentItem
} from '@/service/api';

const message = useMessage();
const { hasAuth } = useAuth();
const appStore = useAppStore();

const ACCEPT = '.pdf,.xlsx,.docx,.csv,.html,.htm,.md,.txt,.json';
const MAX_MB = 20;

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

// 自定义上传: 交给后端 multipart 接口, 成功后刷新列表
const uploading = ref(false);
async function handleCustomRequest({ file }: UploadCustomRequestOptions) {
  const raw = file.file;
  if (!raw) return;
  if (raw.size > MAX_MB * 1024 * 1024) {
    message.warning(`文件超过大小上限 ${MAX_MB}MB`);
    return;
  }
  uploading.value = true;
  try {
    const { data, error } = await fetchUploadDocument(raw);
    if (error || !data) {
      message.error(error?.message || '上传失败');
      return;
    }
    message.success(data.parse_status === 'ok' ? '上传并解析成功' : '上传成功 (解析未完成, 详情见列表)');
    page.current = 1;
    await loadList();
  } finally {
    uploading.value = false;
  }
}
function handleBeforeUpload({ file }: { file: UploadFileInfo }) {
  const name = file.name || '';
  const ok = ACCEPT.split(',').some(ext => name.toLowerCase().endsWith(ext.slice(1)));
  if (!ok) {
    message.warning(`不支持的文件类型 (支持: pdf/xlsx/docx/csv/html/md/txt/json)`);
    return false;
  }
  return true;
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

onMounted(() => {
  loadList();
});
</script>

<template>
  <div class="min-h-full p-2" :class="appStore.isMobile ? '' : 'p-4'">
    <NCard :bordered="false" size="small" title="文档管理">
      <template #header-extra>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-400 hidden sm:inline">支持 pdf/xlsx/docx/csv/html/md/txt/json, 单文件 ≤ {{ MAX_MB }}MB; AI 可通过文档工具读取</span>
          <NUpload
            v-if="hasAuth('document:upload')"
            :custom-request="handleCustomRequest"
            :before-upload="handleBeforeUpload"
            :accept="ACCEPT"
            :max="5"
            :show-file-list="false"
          >
            <NButton size="small" type="primary" :loading="uploading">
              <template #icon><SvgIcon icon="mdi:upload" /></template>
              上传文档
            </NButton>
          </NUpload>
        </div>
      </template>

      <div class="min-h-48">
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
              <span>{{ doc.created_at?.slice(0, 16).replace('T', ' ') }}</span>
            </div>
          </div>
          <div class="flex items-center gap-1 shrink-0">
            <NButton v-if="doc.parse_status === 'ok'" size="tiny" quaternary @click="openPreview(doc)">预览</NButton>
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
        <NPagination v-model:page="page.current" v-model:page-size="page.size" :item-count="total" @update:page="loadList" />
      </div>
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
