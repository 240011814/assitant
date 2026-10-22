<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref } from 'vue';
import QRCode from 'qrcode';
import {
  NAlert,
  NButton,
  NCard,
  NDescriptions,
  NDescriptionsItem,
  NList,
  NListItem,
  NModal,
  NPopconfirm,
  NSpace,
  NSpin,
  NTag,
  useMessage
} from 'naive-ui';
import {
  getMihoyoBindStatus,
  getMihoyoQrStatus,
  runMihoyoCheckin,
  startMihoyoQrLogin,
  unbindMihoyo
} from '@/service/api';

defineOptions({ name: 'MihoyoCheckinPane' });

const message = useMessage();

// ---------- 绑定状态 ----------
const loading = ref(false);
const bindStatus = ref<Api.Mihoyo.BindStatus | null>(null);

async function loadBindStatus() {
  loading.value = true;
  const { data, error } = await getMihoyoBindStatus();
  if (!error && data) {
    bindStatus.value = data;
  }
  loading.value = false;
}

async function handleUnbind() {
  const { error } = await unbindMihoyo();
  if (error) return;
  message.success('已解绑米游社账号');
  bindStatus.value = null;
  await loadBindStatus();
}

// ---------- 签到 ----------
const checking = ref(false);
const lastResults = ref<Api.Mihoyo.CheckinResult[]>([]);

const statusTagType: Record<string, 'success' | 'info' | 'warning' | 'error' | 'default'> = {
  success: 'success',
  already: 'info',
  blocked: 'warning',
  failed: 'error',
  no_role: 'default'
};
const statusText: Record<string, string> = {
  success: '成功',
  already: '已签',
  blocked: '验证码',
  failed: '失败',
  no_role: '无角色'
};

function renderStatusTag(status: string) {
  return h(NTag, { size: 'small', type: statusTagType[status] ?? 'default' }, { default: () => statusText[status] ?? status });
}

async function handleCheckin() {
  checking.value = true;
  try {
    const { data, error } = await runMihoyoCheckin();
    if (error || !data) return;
    lastResults.value = data;
    const ok = data.every((r) => r.status === 'success' || r.status === 'already' || r.status === 'no_role');
    message.success(ok ? '签到完成' : '签到完成, 部分角色未签上 (见详情)');
  } finally {
    checking.value = false;
  }
}

// ---------- 扫码绑定 ----------
const qrModalShow = ref(false);
const qrDataUrl = ref('');
const qrTicket = ref('');
const qrStatus = ref<'idle' | 'creating' | 'waiting' | 'scanned' | 'confirmed' | 'expired'>('idle');
const qrLoading = ref(false);
let qrPollTimer: ReturnType<typeof setTimeout> | null = null;

const qrStatusTip = computed(() => {
  switch (qrStatus.value) {
    case 'creating':
      return '正在生成二维码...';
    case 'waiting':
      return '等待扫描: 请用米游社 App 扫一扫';
    case 'scanned':
      return '已扫描: 请在手机上点【确认登录】';
    case 'confirmed':
      return '绑定成功 ✅';
    case 'expired':
      return '二维码已过期, 请重新获取';
    default:
      return '';
  }
});

function stopQrPolling() {
  if (qrPollTimer) {
    clearTimeout(qrPollTimer);
    qrPollTimer = null;
  }
}

function openQrModal() {
  qrDataUrl.value = '';
  qrTicket.value = '';
  qrStatus.value = 'idle';
  qrModalShow.value = true;
  handleCreateQr();
}

function closeQrModal() {
  stopQrPolling();
  qrModalShow.value = false;
}

async function handleCreateQr() {
  stopQrPolling();
  qrStatus.value = 'creating';
  qrLoading.value = true;
  qrDataUrl.value = '';
  qrTicket.value = '';
  try {
    const { data, error } = await startMihoyoQrLogin();
    if (error || !data) {
      qrStatus.value = 'idle';
      return;
    }
    // 本地生成二维码图片, 扫码内容不发第三方
    qrDataUrl.value = await QRCode.toDataURL(data.qrUrl, { width: 260, margin: 1 });
    qrTicket.value = data.ticket;
    qrStatus.value = 'waiting';
    scheduleQrPoll();
  } finally {
    qrLoading.value = false;
  }
}

function scheduleQrPoll() {
  stopQrPolling();
  qrPollTimer = setTimeout(async () => {
    if (!qrTicket.value || !qrModalShow.value) return;
    const { data, error } = await getMihoyoQrStatus(qrTicket.value);
    if (error || !data) {
      // 网络抖动继续轮询
      scheduleQrPoll();
      return;
    }
    switch (data.status) {
      case 'Scanned':
        qrStatus.value = 'scanned';
        scheduleQrPoll();
        break;
      case 'Confirmed':
        qrStatus.value = 'confirmed';
        stopQrPolling();
        message.success(`米游社账号绑定成功: ${data.nickname || data.uid || ''}`);
        await loadBindStatus();
        setTimeout(closeQrModal, 800);
        break;
      case 'Expired':
        qrStatus.value = 'expired';
        stopQrPolling();
        break;
      default:
        scheduleQrPoll();
    }
  }, 2000);
}

onBeforeUnmount(() => {
  stopQrPolling();
});

loadBindStatus();
</script>

<template>
  <NSpace vertical :size="16">
    <NCard size="small" :bordered="false" embedded>
      <NSpin :show="loading">
        <NDescriptions v-if="bindStatus?.bound" :column="2" label-placement="left" size="small">
          <NDescriptionsItem label="状态">
            <NTag size="small" type="success">已绑定</NTag>
          </NDescriptionsItem>
          <NDescriptionsItem label="昵称">{{ bindStatus?.nickname || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="UID">{{ bindStatus?.uid || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="说明">每天 8:23 自动签到原神</NDescriptionsItem>
        </NDescriptions>
        <NAlert v-else type="info" title="米游社未绑定">
          扫码登录米游社 App 后, 后台每天定时自动签到原神, 无需保存游戏密码。
        </NAlert>
      </NSpin>

      <NSpace class="mt-16px">
        <NButton size="small" :disabled="qrLoading" @click="openQrModal">
          {{ bindStatus?.bound ? '重新扫码绑定' : '扫码绑定' }}
        </NButton>
        <NPopconfirm v-if="bindStatus?.bound" @positive-click="handleUnbind">
          <template #trigger>
            <NButton size="small" quaternary type="error">解绑</NButton>
          </template>
          解绑后停用每日签到, 确认删除本账号登录态?
        </NPopconfirm>
      </NSpace>
    </NCard>

    <NCard size="small" :bordered="false" embedded title="签到">
      <NButton type="primary" size="small" :loading="checking" :disabled="!bindStatus?.bound" @click="handleCheckin">
        立即签到
      </NButton>

      <NList v-if="lastResults.length" class="mt-12px">
        <NListItem v-for="res in lastResults" :key="res.gameUid || res.message">
          <NSpace align="center">
            <component :is="renderStatusTag(res.status)" />
            <span>{{ res.message }}</span>
          </NSpace>
        </NListItem>
      </NList>
    </NCard>

    <NModal v-model:show="qrModalShow" preset="card" title="扫码绑定米游社" class="w-340px" @after-leave="closeQrModal">
      <div class="flex flex-col items-center gap-12px">
        <div v-if="qrDataUrl" class="relative">
          <img :src="qrDataUrl" alt="米游社扫码登录" class="w-260px h-260px" />
          <div
            v-if="qrStatus === 'expired'"
            class="absolute inset-0 flex flex-col items-center justify-center bg-white/90 gap-8px"
          >
            <span class="text-14px">二维码已过期</span>
            <NButton size="small" type="primary" @click="handleCreateQr">重新获取</NButton>
          </div>
        </div>
        <div v-else class="w-260px h-260px flex flex-col items-center justify-center gap-8px">
          <NSpin v-if="qrStatus === 'creating'" />
        </div>

        <div v-if="qrStatusTip" class="text-14px" :class="{ 'text-success font-bold': qrStatus === 'confirmed' }">
          {{ qrStatusTip }}
        </div>
        <div class="text-12px text-gray-400 leading-relaxed text-center">
          打开【米游社 App】→ 扫一扫 → 手机上点【确认登录】
        </div>
      </div>
    </NModal>
  </NSpace>
</template>
