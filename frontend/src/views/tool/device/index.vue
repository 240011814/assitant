<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import { useAppStore } from '@/store/modules/app'
import hljs from 'highlight.js/lib/common'
import { useAuth } from '@/hooks/business/auth'
import {
  clearWolRelay,
  connectWol,
  deleteWolRelay,
  disconnectWol,
  getWolDevices,
  getWolMacList,
  getWolStatus,
  otaWolDevice,
  publishWolRaw,
  updateWolMac,
  wakeWolDevice
} from '@/service/api'
import { $t } from '@/locales'

defineOptions({ name: 'ToolDevice' })

const message = useMessage()
const appStore = useAppStore()
const { hasAuth } = useAuth()

const canManage = computed(() => hasAuth('tool:device:manage'))

const statusInfo = ref<Api.Wol.Status | null>(null)
const devices = ref<Api.Wol.Device[]>([])
const connecting = ref(false)
const discovering = ref(false)

const connected = computed(() => Boolean(statusInfo.value?.connected))

// 地址簿弹窗
const macModalVisible = ref(false)
const activeDevice = ref<Api.Wol.Device | null>(null)
const macList = ref<Api.Wol.MacEntry[]>([])
const macLoading = ref(false)
const macForm = reactive({ mac: '', ip: '' })

// 唤醒弹窗
const wakeModalVisible = ref(false)
const wakeForm = reactive({ mac: '', ip: '' })

// 调试弹窗
const debugModalVisible = ref(false)
const debugForm = reactive({ topic: '', payload: '' })

// OTA 升级弹窗 (状态/进度来自设备状态快照的 version + ota 字段, 随 5s 轮询刷新)
const otaModalVisible = ref(false)
const otaSending = ref(false)
const otaCmd = ref<'check' | 'upgrade' | 'status'>('status')
// 优先取轮询里的最新对象, 保证弹窗内的进度/状态是活的
const otaDevice = computed(() => devices.value.find(d => d.id === activeDevice.value?.id) ?? activeDevice.value)
const otaInProgress = computed(() =>
  ['checking', 'downloading', 'rebooting'].includes(otaDevice.value?.ota?.state ?? '')
)

const macReg = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/i

function formatTime(value: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

function rssiTag(rssi: number): 'success' | 'warning' | 'error' {
  if (rssi >= -55) return 'success'
  if (rssi >= -70) return 'warning'
  return 'error'
}

async function loadStatus() {
  const { data } = await getWolStatus()
  if (data) statusInfo.value = data
}

async function loadDevices(discover: boolean) {
  if (discover) discovering.value = true
  const { data } = await getWolDevices(discover)
  if (data) devices.value = data
  if (discover) discovering.value = false
}

async function handleConnect() {
  connecting.value = true
  const { data } = await connectWol()
  connecting.value = false
  if (data) {
    statusInfo.value = data
    message.success($t('page.tool.device.connectSuccess'))
    // 连接成功后自动发现设备
    await loadDevices(true)
  }
}

async function handleDisconnect() {
  const { data } = await disconnectWol()
  if (data) {
    statusInfo.value = data
    devices.value = []
    message.success($t('page.tool.device.disconnectSuccess'))
  }
}

function openMacModal(device: Api.Wol.Device) {
  activeDevice.value = device
  macForm.mac = ''
  macForm.ip = ''
  macModalVisible.value = true
  refreshMacList(device.id)
}

async function refreshMacList(deviceId: string) {
  macLoading.value = true
  const { data } = await getWolMacList(deviceId)
  if (data) macList.value = data
  macLoading.value = false
}

async function submitMac() {
  if (!activeDevice.value) return
  if (!macReg.test(macForm.mac.trim())) {
    message.warning($t('page.tool.device.macInvalid'))
    return
  }
  const { error } = await updateWolMac(activeDevice.value.id, {
    action: 'set',
    mac: macForm.mac.trim().toUpperCase(),
    ip: macForm.ip.trim()
  })
  if (!error) {
    message.success($t('page.tool.device.macSaved'))
    macForm.mac = ''
    macForm.ip = ''
    await refreshMacList(activeDevice.value.id)
  }
}

async function deleteMac(entry: Api.Wol.MacEntry) {
  if (!activeDevice.value) return
  const { error } = await updateWolMac(activeDevice.value.id, { action: 'del', mac: entry.mac })
  if (!error) {
    message.success($t('page.tool.device.deleted'))
    await refreshMacList(activeDevice.value.id)
  }
}

async function clearMacList() {
  if (!activeDevice.value) return
  const { error } = await updateWolMac(activeDevice.value.id, { action: 'clear' })
  if (!error) {
    message.success($t('page.tool.device.cleared'))
    await refreshMacList(activeDevice.value.id)
  }
}

async function openWakeModal(device: Api.Wol.Device) {
  activeDevice.value = device
  wakeForm.mac = ''
  wakeForm.ip = ''
  wakeModalVisible.value = true
  const { data } = await getWolMacList(device.id)
  if (data) macList.value = data
}

function onWakeMacChange(value: unknown) {
  const hit = macList.value.find(item => item.mac === value)
  if (hit && hit.ip) wakeForm.ip = hit.ip
}

async function sendWake() {
  if (!activeDevice.value) return
  if (!macReg.test(wakeForm.mac.trim())) {
    message.warning($t('page.tool.device.macInvalid'))
    return
  }
  const { error } = await wakeWolDevice(activeDevice.value.id, {
    mac: wakeForm.mac.trim().toUpperCase(),
    ip: wakeForm.ip.trim()
  })
  if (!error) {
    message.success($t('page.tool.device.wakeSent'))
    wakeModalVisible.value = false
  }
}

function openOtaModal(device: Api.Wol.Device) {
  activeDevice.value = device
  otaModalVisible.value = true
}

function otaStateLabel(state?: string) {
  if (!state) return '-'
  const map: Record<string, string> = {
    idle: $t('page.tool.device.otaStateIdle'),
    checking: $t('page.tool.device.otaStateChecking'),
    update_available: $t('page.tool.device.otaStateUpdateAvailable'),
    downloading: $t('page.tool.device.otaStateDownloading'),
    rebooting: $t('page.tool.device.otaStateRebooting'),
    failed: $t('page.tool.device.otaStateFailed')
  }
  return map[state] ?? state
}

function otaStateTag(state?: string): 'default' | 'success' | 'warning' | 'info' | 'error' {
  switch (state) {
    case 'idle':
      return 'success'
    case 'update_available':
      return 'warning'
    case 'checking':
    case 'downloading':
    case 'rebooting':
      return 'info'
    case 'failed':
      return 'error'
    default:
      return 'default'
  }
}

async function sendOta(cmd: 'check' | 'upgrade' | 'status') {
  if (!activeDevice.value) return
  // 进行中只禁止重复触发 check/upgrade, 查状态随时可发
  if (cmd !== 'status' && otaInProgress.value) return
  otaCmd.value = cmd
  otaSending.value = true
  const { error } = await otaWolDevice(activeDevice.value.id, { cmd })
  otaSending.value = false
  if (!error) {
    message.success($t('page.tool.device.otaCommandSent'))
    // 设备异步处理后才会发布新状态快照, 稍等再拉一次设备列表
    setTimeout(() => loadDevices(false), 1000)
  }
}

/** 卡片上的 OTA 徽标: 有新版本/进行中/失败时才显示 (空闲不显示) */
function otaBadge(device: Api.Wol.Device): { show: boolean; type: 'warning' | 'info' | 'error'; text: string } {
  const ota = device.ota
  if (ota?.state === 'update_available') {
    return { show: true, type: 'warning', text: `${$t('page.tool.device.otaNewVersion')}: ${ota.latest || '-'}` }
  }
  if (ota?.state === 'failed') {
    return { show: true, type: 'error', text: $t('page.tool.device.otaStateFailed') }
  }
  if (ota && ['checking', 'downloading', 'rebooting'].includes(ota.state)) {
    const progress = ota.progress != null ? ` ${ota.progress}%` : ''
    return { show: true, type: 'info', text: `${otaStateLabel(ota.state)}${progress}` }
  }
  return { show: false, type: 'info', text: '' }
}

function openDebugModal(device: Api.Wol.Device) {
  activeDevice.value = device
  debugForm.topic = `wakemanager/cmd/${device.id}`
  debugForm.payload = '{"cmd":"wake","mac":"","ip":""}'
  debugModalVisible.value = true
}

async function sendDebug() {
  if (!debugForm.topic.trim()) {
    message.warning($t('page.tool.device.topicRequired'))
    return
  }
  if (payloadError.value) {
    message.warning($t('page.tool.device.payloadInvalid'))
    return
  }
  const { error } = await publishWolRaw({ topic: debugForm.topic.trim(), payload: debugForm.payload })
  if (!error) message.success($t('page.tool.device.sent'))
}

// 中转站 (订阅 texthub/+, 报文 {"deviceId","text"}, 最多 50 条 FIFO; 发送方 ID 即主题最后一级 texthub/<ID>)
const relaySender = ref('web')
const relaySenderValid = computed(() => {
  const id = relaySender.value.trim()
  return id.length > 0 && !/[+/#\s]/.test(id)
})
const relayVisible = ref(false)
const relayList = computed(() => [...(statusInfo.value?.relay || [])].reverse())
const relayText = ref('')
const relaySending = ref(false)

async function sendRelay() {
  const sender = relaySender.value.trim()
  const text = relayText.value.trim()
  if (!sender || !text) return
  if (!relaySenderValid.value) {
    message.warning($t('page.tool.device.relaySenderInvalid'))
    return
  }
  relaySending.value = true
  const { error } = await publishWolRaw({
    topic: `texthub/${sender}`,
    payload: JSON.stringify({ deviceId: sender, text })
  })
  relaySending.value = false
  if (!error) {
    relayText.value = ''
    message.success($t('page.tool.device.relaySent'))
    // 消息经订阅回流入库后, 立即拉一次状态刷新列表
    await loadStatus()
  }
}

async function copyRelay(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success($t('page.tool.device.relayCopied'))
  } catch {
    message.error($t('page.tool.device.relayCopyFailed'))
  }
}

async function deleteRelayItem(id: number) {
  const { error } = await deleteWolRelay(id)
  if (!error) await loadStatus()
}

async function clearRelayAll() {
  const { error } = await clearWolRelay()
  if (!error) {
    message.success($t('page.tool.device.relayCleared'))
    await loadStatus()
  }
}

// 最近报文 (倒序, JSON 报文格式化 + 高亮)
function prettyJson(raw: string): string | null {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return null
  }
}

const debugReports = computed(() =>
  [...(statusInfo.value?.messages || [])].reverse().map(msg => {
    const pretty = prettyJson(msg.payload)
    return {
      ...msg,
      pretty,
      html: pretty ? hljs.highlight(pretty, { language: 'json', ignoreIllegals: true }).value : ''
    }
  })
)

// 载入报文到发送区编辑重发
function editReport(msg: { topic: string; pretty: string | null; payload: string }) {
  debugForm.topic = msg.topic
  debugForm.payload = msg.pretty ?? msg.payload
}

// 发送区 JSON 实时校验
const payloadError = computed(() => Boolean(debugForm.payload.trim()) && prettyJson(debugForm.payload) === null)

function formatPayload() {
  const pretty = prettyJson(debugForm.payload)
  if (pretty === null) {
    message.warning($t('page.tool.device.payloadInvalid'))
    return
  }
  debugForm.payload = pretty
}

const macOptions = computed(() => macList.value.map(item => ({ label: item.mac, value: item.mac })))

function macRowKey(row: Api.Wol.MacEntry) {
  return row.mac
}

const macColumns = [
  { title: $t('page.tool.device.mac'), key: 'mac', minWidth: 160 },
  { title: 'IP', key: 'ip', minWidth: 140 },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 90,
    render: (row: Api.Wol.MacEntry) =>
      canManage.value
        ? h(
            NButton,
            { size: 'tiny', text: true, type: 'error', onClick: () => deleteMac(row) },
            { default: () => $t('common.delete') }
          )
        : '-'
  }
]

let timer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await loadStatus()
  // MQTT 已连接时打开页面自动发现设备一次 (广播 status 后等设备上报)
  if (connected.value) {
    await loadDevices(true)
  }
  timer = setInterval(async () => {
    await loadStatus()
    if (connected.value) await loadDevices(false)
  }, 5000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- 顶部工具栏: 标题 + 连接状态 + 操作 -->
    <div class="flex flex-wrap items-center gap-3 border-b border-gray-200 px-4 py-3">
      <span class="text-lg font-bold">{{ $t('page.tool.device.devices') }}</span>
      <NTag :type="connected ? 'success' : 'error'" size="small" :bordered="false">
        {{ connected ? $t('page.tool.device.connected') : $t('page.tool.device.disconnected') }}
      </NTag>
      <span v-if="statusInfo?.host" class="text-sm text-gray-500">
        {{ statusInfo.host }}:{{ statusInfo.port }}
      </span>
      <span class="text-sm text-gray-500">
        {{ $t('page.tool.device.deviceCount') }}: {{ statusInfo?.device_count ?? 0 }}
      </span>
      <div class="ml-auto flex items-center gap-2">
        <NButton size="small" secondary @click="relayVisible = true">
          {{ $t('page.tool.device.relay') }} ({{ relayList.length }})
        </NButton>
        <NButton v-if="canManage && !connected" type="primary" :loading="connecting" @click="handleConnect">
          {{ $t('page.tool.device.connect') }}
        </NButton>
        <NButton v-if="canManage && connected" type="warning" secondary @click="handleDisconnect">
          {{ $t('page.tool.device.disconnect') }}
        </NButton>
        <NButton v-if="connected" type="primary" :loading="discovering" @click="loadDevices(true)">
          {{ $t('page.tool.device.refresh') }}
        </NButton>
      </div>
    </div>

    <NAlert v-if="statusInfo?.last_error" type="error" class="m-3" :show-icon="true">
      {{ statusInfo.last_error }}
    </NAlert>

    <!-- 设备卡片 -->
    <div class="flex-1 overflow-auto p-4">
      <div v-if="devices.length" class="grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
        <NCard v-for="device in devices" :key="device.id" class="h-full">
          <template #header>
            <span class="font-mono text-base">{{ device.id }}</span>
          </template>
          <template #header-extra>
            <NSpace :size="4">
              <NTag size="small" :type="rssiTag(device.rssi)" :bordered="false">{{ device.rssi }} dBm</NTag>
              <NTag size="small" :type="device.mqtt_connected ? 'success' : 'error'" :bordered="false">
                {{ device.mqtt_connected ? $t('page.tool.device.online') : $t('page.tool.device.offline') }}
              </NTag>
            </NSpace>
          </template>
          <div class="space-y-1.5 text-sm">
            <div><span class="text-gray-400">IP: </span>{{ device.ip || '-' }}</div>
            <div><span class="text-gray-400">WiFi: </span>{{ device.ssid || '-' }}</div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.temp') }}: </span>
              {{ device.temp_c ? `${device.temp_c.toFixed(2)} °C` : '-' }}
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.heap') }}: </span>
              {{ device.heap ? `${(device.heap / 1024).toFixed(1)} KB` : '-' }}
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.broadcast') }}: </span>{{ device.broadcast || '-' }}
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.macCount') }}: </span>{{ device.mac_count }}
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.version') }}: </span>
              {{ device.version || '-' }}
              <NTag
                v-if="otaBadge(device).show"
                size="tiny"
                :bordered="false"
                :type="otaBadge(device).type"
                class="ml-1"
              >
                {{ otaBadge(device).text }}
              </NTag>
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.bootTime') }}: </span>{{ device.boot_time || '-' }}
            </div>
            <div>
              <span class="text-gray-400">{{ $t('page.tool.device.lastSeen') }}: </span>
              {{ formatTime(device.last_seen) }}
            </div>
          </div>
          <div class="mt-3 flex gap-2">
            <NButton size="small" @click="openMacModal(device)">{{ $t('page.tool.device.addressBook') }}</NButton>
            <NButton v-if="canManage" size="small" type="primary" @click="openWakeModal(device)">
              {{ $t('page.tool.device.wake') }}
            </NButton>
            <NButton v-if="canManage" size="small" type="warning" secondary @click="openOtaModal(device)">
              {{ $t('page.tool.device.ota') }}
            </NButton>
            <NButton size="small" secondary @click="openDebugModal(device)">
              {{ $t('page.tool.device.debug') }}
            </NButton>
          </div>
        </NCard>
      </div>
      <div v-else class="mt-20 flex flex-col items-center gap-3">
        <NEmpty
          :description="
            connected ? $t('page.tool.device.empty') : $t('page.tool.device.connectFirst')
          "
        />
        <NButton v-if="!connected && canManage" type="primary" :loading="connecting" @click="handleConnect">
          {{ $t('page.tool.device.connect') }}
        </NButton>
      </div>
    </div>

    <!-- 地址簿弹窗 -->
    <NModal v-model:show="macModalVisible" preset="card" style="width: 640px" class="max-w-[96vw]" :title="$t('page.tool.device.macTitle')">
      <div class="mb-3 text-sm text-gray-500">
        {{ $t('page.tool.device.id') }}: <strong>{{ activeDevice?.id }}</strong>
      </div>
      <div v-if="canManage" class="mb-3 flex items-end gap-2">
        <NFormItem :label="$t('page.tool.device.mac')" class="flex-1" :show-feedback="false">
          <NInput v-model:value="macForm.mac" :placeholder="$t('page.tool.device.macPlaceholder')" clearable />
        </NFormItem>
        <NFormItem label="IP" class="flex-1" :show-feedback="false">
          <NInput v-model:value="macForm.ip" :placeholder="$t('page.tool.device.ipPlaceholder')" clearable />
        </NFormItem>
        <NButton type="primary" @click="submitMac">{{ $t('page.tool.device.add') }}</NButton>
        <NButton @click="refreshMacList(activeDevice?.id || '')">{{ $t('page.tool.device.refreshMac') }}</NButton>
      </div>
      <NDataTable
        :columns="macColumns"
        :data="macList"
        :loading="macLoading"
        :row-key="macRowKey"
        :scroll-x="400"
        size="small"
      >
        <template #empty>
          <NEmpty :description="$t('page.tool.device.macEmpty')" />
        </template>
      </NDataTable>
      <template #footer>
        <div class="flex justify-between">
          <NPopconfirm v-if="canManage" @positive-click="clearMacList">
            <template #trigger>
              <NButton type="error" secondary>{{ $t('page.tool.device.clear') }}</NButton>
            </template>
            {{ $t('page.tool.device.clearConfirm') }}
          </NPopconfirm>
          <NButton @click="macModalVisible = false">{{ $t('common.close') }}</NButton>
        </div>
      </template>
    </NModal>

    <!-- 唤醒弹窗 -->
    <NModal v-model:show="wakeModalVisible" preset="card" style="width: 480px" class="max-w-[96vw]" :title="$t('page.tool.device.wakeTitle')">
      <NForm label-placement="top">
        <NFormItem :label="$t('page.tool.device.mac')">
          <NSelect
            v-model:value="wakeForm.mac"
            :options="macOptions"
            filterable
            tag
            :placeholder="$t('page.tool.device.macPlaceholder')"
            @update:value="onWakeMacChange"
          />
        </NFormItem>
        <NFormItem :label="$t('page.tool.device.wakeIp')">
          <NInput v-model:value="wakeForm.ip" :placeholder="$t('page.tool.device.wakeIpPlaceholder')" clearable />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="wakeModalVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" @click="sendWake">{{ $t('page.tool.device.wakeSend') }}</NButton>
        </div>
      </template>
    </NModal>

    <!-- OTA 升级弹窗 (状态来自设备状态快照的 version + ota 字段) -->
    <NModal v-model:show="otaModalVisible" preset="card" style="width: 520px" class="max-w-[96vw]" :title="$t('page.tool.device.otaTitle')">
      <div class="space-y-2 text-sm">
        <div>
          <span class="text-gray-400">{{ $t('page.tool.device.id') }}: </span>
          <strong class="font-mono">{{ otaDevice?.id || '-' }}</strong>
        </div>
        <div>
          <span class="text-gray-400">{{ $t('page.tool.device.version') }}: </span>
          {{ otaDevice?.ota?.current || otaDevice?.version || '-' }}
        </div>
        <div>
          <span class="text-gray-400">{{ $t('page.tool.device.otaLatest') }}: </span>
          {{ otaDevice?.ota?.latest || '-' }}
        </div>
        <div class="flex items-center gap-2">
          <span class="text-gray-400">{{ $t('page.tool.device.otaState') }}: </span>
          <NTag size="small" :bordered="false" :type="otaStateTag(otaDevice?.ota?.state)">
            {{ otaStateLabel(otaDevice?.ota?.state) }}
          </NTag>
        </div>
        <NProgress
          v-if="otaDevice?.ota?.progress != null"
          type="line"
          :percentage="otaDevice.ota.progress"
          :status="otaDevice.ota.state === 'failed' ? 'error' : 'default'"
        />
        <NAlert v-if="otaDevice?.ota?.error" type="error" :show-icon="true">
          {{ otaDevice.ota.error }}
        </NAlert>
        <div class="text-xs text-gray-400">{{ $t('page.tool.device.otaHint') }}</div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton
            :loading="otaSending && otaCmd === 'status'"
            @click="sendOta('status')"
          >
            {{ $t('page.tool.device.otaRefresh') }}
          </NButton>
          <NButton
            :disabled="otaInProgress"
            :loading="otaSending && otaCmd === 'check'"
            @click="sendOta('check')"
          >
            {{ $t('page.tool.device.otaCheck') }}
          </NButton>
          <NButton
            type="primary"
            :disabled="otaInProgress"
            :loading="otaSending && otaCmd === 'upgrade'"
            @click="sendOta('upgrade')"
          >
            {{ $t('page.tool.device.otaUpgrade') }}
          </NButton>
        </div>
      </template>
    </NModal>

    <!-- 调试弹窗 -->
    <NModal
      v-model:show="debugModalVisible"
      preset="card"
      style="width: 760px"
      class="max-w-[96vw]"
      :title="$t('page.tool.device.debugTitle')"
    >
      <div class="mb-2 text-sm text-gray-500">
        {{ $t('page.tool.device.id') }}: <strong>{{ activeDevice?.id }}</strong>
      </div>

      <div class="mb-1 flex items-center justify-between">
        <span class="font-bold">{{ $t('page.tool.device.reports') }}</span>
        <NButton size="tiny" quaternary :loading="discovering" @click="loadStatus">
          {{ $t('page.tool.device.refreshMac') }}
        </NButton>
      </div>
      <div class="mb-3 max-h-60 overflow-y-auto rounded border border-gray-200 p-2">
        <div v-if="!debugReports.length" class="text-sm text-gray-400">
          {{ $t('page.tool.device.reportsEmpty') }}
        </div>
        <div
          v-for="(msg, index) in debugReports"
          :key="index"
          class="border-b border-gray-100 py-1.5 text-xs last:border-0"
        >
          <div class="flex flex-wrap items-center gap-1 text-gray-400">
            <span>{{ formatTime(msg.received_at) }}</span>
            <NTag
              size="tiny"
              :bordered="false"
              :type="msg.direction === 'send' ? 'warning' : 'info'"
            >
              {{ msg.direction === 'send' ? $t('page.tool.device.dirSend') : $t('page.tool.device.dirRecv') }}
            </NTag>
            <span class="break-all text-gray-500">{{ msg.topic || '-' }}</span>
            <span>{{ msg.device_id || '-' }} · {{ msg.cmd || '-' }}</span>
            <NButton size="tiny" quaternary type="primary" class="ml-auto" @click="editReport(msg)">
              {{ $t('page.tool.device.editSend') }}
            </NButton>
          </div>
          <pre
            v-if="msg.html"
            class="hljs mt-1 max-h-40 overflow-auto rounded !p-2 text-xs"
            v-html="msg.html"
          ></pre>
          <div v-else class="mt-1 whitespace-pre-wrap break-all text-gray-700">{{ msg.payload }}</div>
        </div>
      </div>

      <NDivider class="!my-3" />

      <NForm label-placement="top" :show-feedback="false">
        <NFormItem :label="$t('page.tool.device.topic')">
          <NInput v-model:value="debugForm.topic" :placeholder="$t('page.tool.device.topicPlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.tool.device.payload')">
          <div class="w-full">
            <NInput
              v-model:value="debugForm.payload"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 8 }"
              :status="payloadError ? 'error' : undefined"
              :placeholder="$t('page.tool.device.payloadPlaceholder')"
            />
            <div class="mt-1 flex items-center justify-between">
              <span v-if="payloadError" class="text-xs text-red-500">
                {{ $t('page.tool.device.payloadInvalid') }}
              </span>
              <span v-else class="text-xs text-gray-400">JSON</span>
              <NButton size="tiny" quaternary @click="formatPayload">
                {{ $t('page.tool.device.formatJson') }}
              </NButton>
            </div>
          </div>
        </NFormItem>
      </NForm>

      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="debugModalVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton v-if="canManage" type="primary" @click="sendDebug">{{ $t('page.tool.device.send') }}</NButton>
        </div>
      </template>
    </NModal>

    <!-- 中转站抽屉 (texthub/+, 与设备无关) -->
    <NDrawer v-model:show="relayVisible" :width="appStore.isMobile ? '100%' : 420" placement="right">
      <NDrawerContent
        :title="$t('page.tool.device.relay')"
        closable
        :footer-style="{ padding: '12px 16px' }"
      >
        <div class="mb-1 flex items-start justify-between gap-2">
          <div class="text-xs text-gray-400">{{ $t('page.tool.device.relayDesc') }}</div>
          <NPopconfirm v-if="canManage && relayList.length" @positive-click="clearRelayAll">
            <template #trigger>
              <NButton size="tiny" type="error" secondary>{{ $t('page.tool.device.relayClear') }}</NButton>
            </template>
            {{ $t('page.tool.device.relayClearConfirm') }}
          </NPopconfirm>
        </div>
        <div v-if="relayList.length" class="space-y-2">
          <div v-for="item in relayList" :key="item.id" class="rounded border border-gray-200 p-2">
            <div class="mb-1 flex items-center justify-between text-xs text-gray-400">
              <span>{{ item.device_id || '-' }} · {{ formatTime(item.received_at) }}</span>
              <NSpace :size="4">
                <NButton size="tiny" quaternary type="primary" @click="copyRelay(item.text)">
                  {{ $t('page.tool.device.relayCopy') }}
                </NButton>
                <NButton
                  v-if="canManage"
                  size="tiny"
                  quaternary
                  type="error"
                  @click="deleteRelayItem(item.id)"
                >
                  {{ $t('common.delete') }}
                </NButton>
              </NSpace>
            </div>
            <div class="whitespace-pre-wrap break-all text-sm text-gray-700">{{ item.text }}</div>
          </div>
        </div>
        <NEmpty v-else class="mt-10" :description="$t('page.tool.device.relayEmpty')" />
        <template v-if="canManage" #footer>
          <div class="flex w-full flex-col gap-2">
            <div class="flex items-center gap-2">
              <span class="whitespace-nowrap text-sm text-gray-500">
                {{ $t('page.tool.device.relaySender') }}
              </span>
              <NInput
                v-model:value="relaySender"
                size="small"
                :status="relaySenderValid ? undefined : 'error'"
                :placeholder="$t('page.tool.device.relaySenderPlaceholder')"
              />
            </div>
            <div v-if="!relaySenderValid" class="text-xs text-red-500">
              {{ $t('page.tool.device.relaySenderInvalid') }}
            </div>
            <NInput
              v-model:value="relayText"
              type="textarea"
              :rows="5"
              class="w-full"
              :placeholder="$t('page.tool.device.relayPlaceholder')"
            />
            <div class="flex justify-end">
              <NButton
                type="primary"
                :loading="relaySending"
                :disabled="!relayText.trim() || !relaySenderValid"
                @click="sendRelay"
              >
                {{ $t('page.tool.device.relaySend') }}
              </NButton>
            </div>
          </div>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
