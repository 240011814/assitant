<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import { useAuth } from '@/hooks/business/auth'
import {
  connectWol,
  disconnectWol,
  getWolConfig,
  getWolDevices,
  getWolMacList,
  getWolStatus,
  publishWolRaw,
  updateWolConfig,
  updateWolMac,
  wakeWolDevice
} from '@/service/api'
import { $t } from '@/locales'

defineOptions({ name: 'ToolDevice' })

const message = useMessage()
const { hasAuth } = useAuth()

const canManage = computed(() => hasAuth('tool:device:manage'))

const configForm = reactive({ host: '', port: 1883, username: '', password: '' })
const statusInfo = ref<Api.Wol.Status | null>(null)
const devices = ref<Api.Wol.Device[]>([])
const saving = ref(false)
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

async function loadConfig() {
  const { data } = await getWolConfig()
  if (data) {
    configForm.host = data.host
    configForm.port = data.port || 1883
    configForm.username = data.username
  }
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

async function saveConfig() {
  if (!configForm.host.trim()) {
    message.warning($t('page.tool.device.hostRequired'))
    return
  }
  saving.value = true
  const { data } = await updateWolConfig({ ...configForm })
  saving.value = false
  if (data) {
    configForm.password = ''
    message.success($t('page.tool.device.saved'))
  }
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
  const { error } = await publishWolRaw({ topic: debugForm.topic.trim(), payload: debugForm.payload })
  if (!error) message.success($t('page.tool.device.sent'))
}

// 最近报文 (倒序)
const debugReports = computed(() => [...(statusInfo.value?.messages || [])].reverse())

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
  await loadConfig()
  await loadStatus()
  if (connected.value) {
    await loadDevices(false)
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
  <div class="flex h-full gap-4">
    <!-- 左侧: 连接配置 + 状态 -->
    <div class="w-80 flex-shrink-0 overflow-y-auto">
      <NCard :title="$t('page.tool.device.configTitle')" size="small" class="mb-4">
        <NForm label-placement="top" :show-feedback="false">
          <NFormItem :label="$t('page.tool.device.host')">
            <NInput
              v-model:value="configForm.host"
              :placeholder="$t('page.tool.device.hostPlaceholder')"
              clearable
            />
          </NFormItem>
          <NFormItem :label="$t('page.tool.device.port')">
            <NInputNumber v-model:value="configForm.port" :min="1" :max="65535" class="w-full" />
          </NFormItem>
          <NFormItem :label="`${$t('page.tool.device.username')} (${$t('page.tool.device.optional')})`">
            <NInput v-model:value="configForm.username" clearable />
          </NFormItem>
          <NFormItem :label="$t('page.tool.device.password')">
            <NInput
              v-model:value="configForm.password"
              type="password"
              show-password-on="click"
              :placeholder="$t('page.tool.device.passwordPlaceholder')"
            />
          </NFormItem>
        </NForm>
        <div class="flex gap-2">
          <NButton v-if="canManage" class="flex-1" :loading="saving" @click="saveConfig">
            {{ $t('page.tool.device.save') }}
          </NButton>
          <NButton
            v-if="canManage && !connected"
            class="flex-1"
            type="primary"
            :loading="connecting"
            @click="handleConnect"
          >
            {{ $t('page.tool.device.connect') }}
          </NButton>
          <NButton v-if="canManage && connected" class="flex-1" type="warning" @click="handleDisconnect">
            {{ $t('page.tool.device.disconnect') }}
          </NButton>
        </div>
      </NCard>

      <NCard :title="$t('page.tool.device.statusTitle')" size="small">
        <div class="flex items-center gap-2 mb-2">
          <NTag :type="connected ? 'success' : 'error'" size="small" :bordered="false">
            {{ connected ? $t('page.tool.device.connected') : $t('page.tool.device.disconnected') }}
          </NTag>
          <span v-if="statusInfo?.host" class="text-sm text-gray-500">
            {{ statusInfo.host }}:{{ statusInfo.port }}
          </span>
        </div>
        <div class="text-sm text-gray-500">
          {{ $t('page.tool.device.deviceCount') }}: {{ statusInfo?.device_count ?? 0 }}
        </div>
        <NAlert v-if="statusInfo?.last_error" type="error" class="mt-2" :show-icon="true">
          {{ statusInfo.last_error }}
        </NAlert>
      </NCard>
    </div>

    <!-- 右侧: 设备卡片 -->
    <div class="flex flex-1 flex-col overflow-hidden">
      <div class="flex items-center justify-between border-b border-gray-200 p-3">
        <span class="text-lg font-bold">{{ $t('page.tool.device.devices') }}</span>
        <NButton v-if="connected" type="primary" :loading="discovering" @click="loadDevices(true)">
          {{ $t('page.tool.device.refresh') }}
        </NButton>
      </div>
      <div class="flex-1 overflow-auto p-3">
        <div v-if="devices.length" class="grid grid-cols-1 gap-4 xl:grid-cols-2">
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
                <span class="text-gray-400">{{ $t('page.tool.device.heap') }}: </span>{{ device.heap || '-' }}
              </div>
              <div>
                <span class="text-gray-400">{{ $t('page.tool.device.broadcast') }}: </span>{{ device.broadcast || '-' }}
              </div>
              <div>
                <span class="text-gray-400">{{ $t('page.tool.device.macCount') }}: </span>{{ device.mac_count }}
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
              <NButton size="small" secondary @click="openDebugModal(device)">
                {{ $t('page.tool.device.debug') }}
              </NButton>
            </div>
          </NCard>
        </div>
        <NEmpty v-else class="mt-20" :description="$t('page.tool.device.empty')" />
      </div>
    </div>

    <!-- 地址簿弹窗 -->
    <NModal v-model:show="macModalVisible" preset="card" style="width: 640px" :title="$t('page.tool.device.macTitle')">
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
    <NModal v-model:show="wakeModalVisible" preset="card" style="width: 480px" :title="$t('page.tool.device.wakeTitle')">
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

    <!-- 调试弹窗 -->
    <NModal
      v-model:show="debugModalVisible"
      preset="card"
      style="width: 760px"
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
          class="border-b border-gray-100 py-1 text-xs last:border-0"
        >
          <div class="text-gray-400">
            {{ formatTime(msg.received_at) }} · {{ msg.device_id || '-' }} · {{ msg.cmd || '-' }}
          </div>
          <div class="break-all text-gray-700">{{ msg.payload }}</div>
        </div>
      </div>

      <NDivider class="!my-3" />

      <NForm label-placement="top" :show-feedback="false">
        <NFormItem :label="$t('page.tool.device.topic')">
          <NInput v-model:value="debugForm.topic" :placeholder="$t('page.tool.device.topicPlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.tool.device.payload')">
          <NInput
            v-model:value="debugForm.payload"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :placeholder="$t('page.tool.device.payloadPlaceholder')"
          />
        </NFormItem>
      </NForm>

      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="debugModalVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton v-if="canManage" type="primary" @click="sendDebug">{{ $t('page.tool.device.send') }}</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>