<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import {
  NButton,
  NCard,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSwitch,
  NTag,
  NTooltip,
  useMessage
} from 'naive-ui';
import {
  fetchConnectMCPServer,
  fetchCreateMCPServer,
  fetchDeleteMCPServer,
  fetchMCPServers,
  fetchUpdateMCPServer,
  type MCPServerItem,
  type MCPServerPayload
} from '@/service/api';
import { useAuth } from '@/hooks/business/auth';
import { useAppStore } from '@/store/modules/app';

const message = useMessage();
const { hasAuth } = useAuth();
const appStore = useAppStore();

defineOptions({ name: 'SystemMCP' });

const loading = ref(false);
const servers = ref<MCPServerItem[]>([]);

const statusMap: Record<string, { label: string; type: 'success' | 'error' | 'default' }> = {
  ok: { label: '已连接', type: 'success' },
  failed: { label: '连接失败', type: 'error' },
  none: { label: '未连接', type: 'default' }
};

const transportOptions = [
  { label: 'Streamable HTTP (远程推荐)', value: 'http' },
  { label: 'SSE (远程)', value: 'sse' },
  { label: 'stdio (本地子进程)', value: 'stdio' }
];

async function loadList() {
  loading.value = true;
  try {
    const { data, error } = await fetchMCPServers();
    if (error || !data) {
      message.error(error?.message || '获取服务列表失败');
      return;
    }
    servers.value = data.items;
  } finally {
    loading.value = false;
  }
}

// ============ 编辑弹窗 ============
const showModal = ref(false);
const saving = ref(false);
const editingId = ref<number | null>(null);
const form = reactive<MCPServerPayload>({
  name: '',
  transport: 'http',
  url: '',
  command: '',
  args: '',
  env: '',
  headers: '',
  timeout_seconds: 30,
  enabled: true
});

function openCreate() {
  editingId.value = null;
  Object.assign(form, { name: '', transport: 'http', url: '', command: '', args: '', env: '', headers: '', timeout_seconds: 30, enabled: true });
  showModal.value = true;
}

function openEdit(srv: MCPServerItem) {
  editingId.value = srv.id;
  Object.assign(form, {
    name: srv.name,
    transport: srv.transport,
    url: srv.url,
    command: srv.command,
    args: srv.args,
    env: srv.env,
    headers: srv.headers,
    timeout_seconds: srv.timeout_seconds,
    enabled: srv.enabled
  });
  showModal.value = true;
}

function validateForm(): string | null {
  if (!form.name.trim()) return '请填写服务名称';
  if (form.transport === 'stdio') {
    if (!form.command.trim()) return 'stdio 传输需填写可执行命令';
  } else if (!/^https?:\/\//.test(form.url.trim())) {
    return '远程服务地址需以 http:// 或 https:// 开头';
  }
  for (const [label, val] of [['参数', form.args], ['环境变量', form.env], ['请求头', form.headers]] as const) {
    if (val.trim()) {
      try {
        JSON.parse(val);
      } catch {
        return `${label}不是合法的 JSON`;
      }
    }
  }
  return null;
}

async function handleSave() {
  const err = validateForm();
  if (err) {
    message.warning(err);
    return;
  }
  saving.value = true;
  try {
    const payload: MCPServerPayload = { ...form, name: form.name.trim() };
    const { error } = editingId.value === null ? await fetchCreateMCPServer(payload) : await fetchUpdateMCPServer(editingId.value, payload);
    if (error) {
      message.error(error?.message || '保存失败');
      return;
    }
    message.success(editingId.value === null ? '已创建, 正在连接' : '已保存');
    showModal.value = false;
    // 连接是异步的, 延迟刷新看状态
    await loadList();
    setTimeout(loadList, 3000);
  } finally {
    saving.value = false;
  }
}

async function handleConnect(srv: MCPServerItem) {
  const { error } = await fetchConnectMCPServer(srv.id);
  if (error) {
    message.error(error?.message || '重连失败');
    return;
  }
  message.success(`「${srv.name}」已开始连接`);
  setTimeout(loadList, 3000);
}

async function handleDelete(srv: MCPServerItem) {
  const { error } = await fetchDeleteMCPServer(srv.id);
  if (error) {
    message.error(error?.message || '删除失败');
    return;
  }
  message.success('已删除 (其工具行已同步清理)');
  await loadList();
}

onMounted(() => {
  loadList();
});
</script>

<template>
  <div class="min-h-full p-2" :class="appStore.isMobile ? '' : 'p-4'">
    <NCard :bordered="false" size="small" title="MCP 服务">
      <template #header-extra>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-400 hidden sm:inline">
            发现的工具注册为 mcp_服务名_工具名, 需在「AI 工具管理」页手动新增启用; 删除服务时其工具行自动清理
          </span>
          <NButton v-if="hasAuth('system:mcp:manage')" size="small" type="primary" @click="openCreate">
            <template #icon><SvgIcon icon="mdi:plus" /></template>
            新增服务
          </NButton>
        </div>
      </template>

      <div class="min-h-48">
        <div
          v-for="srv in servers"
          :key="srv.id"
          class="flex items-center gap-3 p-2 rounded border border-transparent hover:bg-gray-100 dark:hover:bg-gray-800 mb-1"
        >
          <SvgIcon icon="mdi:server-network" class="text-20px text-green-500 shrink-0" />
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium truncate">
              {{ srv.name }}
              <NTag :type="srv.enabled ? 'success' : 'default'" size="tiny" :bordered="false" class="ml-1">
                {{ srv.enabled ? '启用' : '禁用' }}
              </NTag>
            </div>
            <div class="text-xs text-gray-400 flex items-center gap-2 mt-0.5 flex-wrap">
              <NTag :type="statusMap[srv.last_status]?.type || 'default'" size="tiny" :bordered="false">
                {{ statusMap[srv.last_status]?.label || srv.last_status }}
              </NTag>
              <span>{{ srv.transport.toUpperCase() }}</span>
              <span>{{ srv.tool_count }} 个工具</span>
              <NTooltip v-if="srv.registered_tools?.length">
                <template #trigger>
                  <span class="cursor-help">工具列表</span>
                </template>
                {{ srv.registered_tools.join(', ') }}
              </NTooltip>
              <NTooltip v-if="srv.last_status === 'failed' && srv.last_error">
                <template #trigger>
                  <span class="text-red-400 cursor-help">失败原因</span>
                </template>
                {{ srv.last_error }}
              </NTooltip>
              <span class="truncate max-w-320px">{{ srv.transport === 'stdio' ? srv.command : srv.url }}</span>
            </div>
          </div>
          <div v-if="hasAuth('system:mcp:manage')" class="flex items-center gap-1 shrink-0">
            <NButton
              size="tiny"
              quaternary
              :disabled="!srv.enabled"
              @click="handleConnect(srv)"
            >
              重连
            </NButton>
            <NButton size="tiny" quaternary @click="openEdit(srv)">编辑</NButton>
            <NPopconfirm @positive-click="handleDelete(srv)">
              <template #trigger>
                <NButton size="tiny" quaternary type="error">删除</NButton>
              </template>
              确认删除「{{ srv.name }}」？其注册的工具与 AI 工具管理中的对应工具行将一并删除。
            </NPopconfirm>
          </div>
        </div>
        <NEmpty v-if="servers.length === 0 && !loading" description="暂无 MCP 服务" class="py-10" />
      </div>
    </NCard>

    <!-- 新增/编辑弹窗 -->
    <NModal v-model:show="showModal" preset="card" :title="editingId === null ? '新增 MCP 服务' : '编辑 MCP 服务'" style="width: 640px">
      <NForm label-placement="left" label-width="110">
        <NFormItem label="服务名称" required>
          <NInput v-model:value="form.name" placeholder="唯一标识, 如 filesystem (工具前缀 mcp_filesystem_)" />
        </NFormItem>
        <NFormItem label="传输方式" required>
          <NSelect v-model:value="form.transport" :options="transportOptions" />
        </NFormItem>
        <NFormItem v-if="form.transport !== 'stdio'" label="服务地址" required>
          <NInput v-model:value="form.url" placeholder="http://127.0.0.1:3000/mcp" />
        </NFormItem>
        <template v-else>
          <NFormItem label="可执行命令" required>
            <NInput v-model:value="form.command" placeholder="npx / uvx / python (需部署机已安装)" />
          </NFormItem>
          <NFormItem label="命令参数">
            <NInput v-model:value="form.args" type="textarea" :rows="2" placeholder='JSON 数组, 如 ["-y", "@modelcontextprotocol/server-filesystem", "/data"]' />
          </NFormItem>
          <NFormItem label="环境变量">
            <NInput v-model:value="form.env" type="textarea" :rows="2" placeholder='JSON 对象, 如 {"API_KEY": "xxx"}' />
          </NFormItem>
        </template>
        <NFormItem v-if="form.transport !== 'stdio'" label="附加请求头">
          <NInput v-model:value="form.headers" type="textarea" :rows="2" placeholder='JSON 对象, 如 {"Authorization": "Bearer xxx"}' />
        </NFormItem>
        <NFormItem label="调用超时(秒)">
          <NInputNumber v-model:value="form.timeout_seconds" :min="5" :max="300" />
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="form.enabled" />
          <span class="text-xs text-gray-400 ml-2">禁用时自动禁用该服务的全部工具</span>
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="handleSave">保存</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
