<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import type { FormInst, FormRules } from 'naive-ui';
import { fetchChangePassword, fetchGetUserProfile, fetchUpdateProfile, fetchGetTelegramConfig, fetchGetTelegramStatus, fetchGenerateTelegramBindCode, fetchUnbindTelegram, fetchGetNotificationPreference, fetchSaveNotificationPreference } from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import AppearanceSettings from '@/layouts/modules/theme-drawer/modules/appearance/index.vue';
import LayoutSettings from '@/layouts/modules/theme-drawer/modules/layout/index.vue';
import GeneralSettings from '@/layouts/modules/theme-drawer/modules/general/index.vue';
import PresetSettings from '@/layouts/modules/theme-drawer/modules/preset/index.vue';
import ConfigOperation from '@/layouts/modules/theme-drawer/modules/config-operation.vue';
import { useClipboard } from '@vueuse/core';
import { useEcharts } from '@/hooks/common/echarts';
import { fetchMyTokenUsage } from '@/service/api';

defineOptions({ name: 'UserProfile' });

const message = useMessage();
const authStore = useAuthStore();
const loading = ref(false);
const profile = ref<Api.Admin.UserProfile | null>(null);
const activeTab = ref('profile');
const themeTab = ref('appearance');

// Telegram binding state
const telegramConfigured = ref(false);
const telegramStatus = ref<Api.Telegram.StatusResponse>({ isBound: false });
const telegramBindCode = ref<Api.Telegram.BindCodeResponse | null>(null);
const telegramLoading = ref(false);

// Notification preference state
const notificationChannels = ref<string[]>(['email']);
const notificationLoading = ref(false);
const savingNotification = ref(false);

const { copy, isSupported } = useClipboard();

// ---------- Token 用量 (切到 Tab 才加载) ----------
const tokenLoaded = ref(false);
const tokenUsage = ref<Api.TokenUsage.MyUsage | null>(null);

const quotaPercent = computed(() => {
  const q = tokenUsage.value?.quota_month;
  if (!q || q <= 0) return null;
  return Math.min(100, Math.round(((tokenUsage.value?.month_used || 0) / q) * 100));
});

const { domRef: tokenChartRef, updateOptions: updateTokenChart } = useEcharts(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '15%', containLabel: true },
  xAxis: { type: 'category', data: [] as string[] },
  yAxis: { type: 'value', minInterval: 1 },
  series: [
    {
      name: 'Token 用量',
      type: 'bar',
      data: [] as number[],
      itemStyle: { color: '#5b8ff9' }
    }
  ]
}));

const fmtNum = (n?: number | null) => (n === undefined || n === null ? '-' : Number(n).toLocaleString('zh-CN'));

async function loadTokenUsage() {
  const { data, error } = await fetchMyTokenUsage();
  if (!error && data) {
    tokenUsage.value = data;
    updateTokenChart(opts => {
      opts.xAxis.data = data.trend.map(i => i.bucket);
      opts.series[0].data = data.trend.map(i => i.total_tokens);
      return opts;
    });
  }
  tokenLoaded.value = true;
}

watch(activeTab, tab => {
  if (tab === 'token' && !tokenLoaded.value) {
    loadTokenUsage();
  }
});

const profileFormRef = ref<FormInst | null>(null);
const passwordFormRef = ref<FormInst | null>(null);

const profileForm = reactive({
  nickname: '',
  email: ''
});

const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
});

const profileRules: FormRules = {
  nickname: [
    {
      required: true,
      message: $t('page.userProfile.nicknameRequired'),
      trigger: ['blur', 'input']
    }
  ]
};

const passwordRules: FormRules = {
  oldPassword: [
    {
      required: true,
      message: $t('page.userProfile.oldPasswordRequired'),
      trigger: ['blur', 'input']
    }
  ],
  newPassword: [
    {
      required: true,
      message: $t('page.userProfile.newPasswordRequired'),
      trigger: ['blur', 'input']
    },
    {
      min: 6,
      message: $t('page.userProfile.passwordMinLength'),
      trigger: ['blur', 'input']
    }
  ],
  confirmPassword: [
    {
      required: true,
      message: $t('page.userProfile.confirmPasswordRequired'),
      trigger: ['blur', 'input']
    },
    {
      validator(_rule: unknown, value: string) {
        return value === passwordForm.newPassword;
      },
      message: $t('page.userProfile.passwordMismatch'),
      trigger: ['blur', 'input']
    }
  ]
};

async function loadProfile() {
  loading.value = true;
  const { data, error } = await fetchGetUserProfile();
  if (!error) {
    profile.value = data;
    profileForm.nickname = data.nickname;
    profileForm.email = data.email || '';
  }
  loading.value = false;
}

async function handleUpdateProfile() {
  await profileFormRef.value?.validate();
  const { error } = await fetchUpdateProfile({ nickname: profileForm.nickname, email: profileForm.email });
  if (!error) {
    message.success($t('page.userProfile.updateSuccess'));
    authStore.userInfo.userName = profile.value?.userName || '';
    await loadProfile();
  }
}

async function handleChangePassword() {
  await passwordFormRef.value?.validate();
  const { error } = await fetchChangePassword({
    oldPassword: passwordForm.oldPassword,
    newPassword: passwordForm.newPassword
  });
  if (!error) {
    message.success($t('page.userProfile.passwordChangeSuccess'));
    authStore.resetStore();
  }
}

// Telegram binding functions
async function loadTelegramConfig() {
  const { data, error } = await fetchGetTelegramConfig();
  if (!error) {
    telegramConfigured.value = data.configured;
    if (data.configured) {
      loadTelegramStatus();
    }
  }
}

async function loadTelegramStatus() {
  const { data, error } = await fetchGetTelegramStatus();
  if (!error) {
    telegramStatus.value = data;
  }
}

async function handleGenerateBindCode() {
  telegramLoading.value = true;
  const { data, error } = await fetchGenerateTelegramBindCode();
  if (!error) {
    telegramBindCode.value = data;
    message.success($t('page.userProfile.telegramGenerateSuccess'));
  }
  telegramLoading.value = false;
}

async function handleUnbindTelegram() {
  telegramLoading.value = true;
  const { error } = await fetchUnbindTelegram();
  if (!error) {
    telegramStatus.value = { isBound: false };
    telegramBindCode.value = null;
    message.success($t('page.userProfile.telegramUnbindSuccess'));
  }
  telegramLoading.value = false;
}

function formatExpireTime(timestamp: number) {
  return new Date(timestamp * 1000).toLocaleString();
}

async function handleCopyBindCode() {
  if (!telegramBindCode.value) return;

  const text = `/bind ${telegramBindCode.value.bindCode}`;

  if (isSupported) {
    await copy(text);
    message.success('已复制到剪贴板');
  } else {
    // Fallback for browsers without clipboard API
    const textarea = document.createElement('textarea');
    textarea.value = text;
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand('copy');
    document.body.removeChild(textarea);
    message.success('已复制到剪贴板');
  }
}

// Notification preference functions
async function loadNotificationPreference() {
  notificationLoading.value = true;
  const { data, error } = await fetchGetNotificationPreference();
  if (!error && data) {
    notificationChannels.value = data;
  }
  notificationLoading.value = false;
}

async function handleSaveNotificationPreference() {
  savingNotification.value = true;
  const { error } = await fetchSaveNotificationPreference(notificationChannels.value);
  if (!error) {
    message.success('通知偏好已保存');
  }
  savingNotification.value = false;
}

// Load profile, telegram config and notification preference on mount
loadProfile();
loadTelegramConfig();
loadNotificationPreference();
</script>

<template>
  <div class="h-full p-6">
    <NSpin :show="loading">
      <div class="flex gap-6 h-full">
        <!-- 左侧：用户信息卡片 -->
        <NCard class="w-320px flex-shrink-0">
          <div class="flex flex-col items-center py-8">
            <NAvatar :size="100" round class="mb-4">
              <SvgIcon icon="ph:user-circle" class="text-60px" />
            </NAvatar>
            <h2 class="text-24px font-bold mb-2">{{ profile?.nickname || profile?.userName }}</h2>
            <NTag type="info">{{ profile?.role }}</NTag>

            <NDivider />

            <NDescriptions :column="1" label-placement="left" class="w-full px-4">
              <NDescriptionsItem :label="$t('page.userProfile.userName')">
                {{ profile?.userName }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.userProfile.nickname')">
                {{ profile?.nickname }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.userProfile.email')">
                {{ profile?.email || '-' }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.userProfile.lastLoginAt')">
                {{ profile?.lastLoginAt || '-' }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.userProfile.createdAt')">
                {{ profile?.createdAt }}
              </NDescriptionsItem>
            </NDescriptions>
          </div>
        </NCard>

        <!-- 右侧：功能区域 -->
        <NCard class="flex-1">
          <NTabs v-model:value="activeTab" type="line" animated>
            <NTabPane name="profile" :tab="$t('page.userProfile.basicInfo')">
              <div class="max-w-600px py-4">
                <NForm
                  ref="profileFormRef"
                  :model="profileForm"
                  :rules="profileRules"
                  label-placement="left"
                  label-width="100"
                >
                  <NFormItem :label="$t('page.userProfile.userName')">
                    <NInput :value="profile?.userName" disabled />
                  </NFormItem>
                  <NFormItem :label="$t('page.userProfile.nickname')" path="nickname">
                    <NInput
                      v-model:value="profileForm.nickname"
                      :placeholder="$t('page.userProfile.nicknamePlaceholder')"
                    />
                  </NFormItem>
                  <NFormItem :label="$t('page.userProfile.email')" path="email">
                    <NInput
                      v-model:value="profileForm.email"
                      :placeholder="$t('page.userProfile.emailPlaceholder')"
                    />
                  </NFormItem>
                  <NFormItem :label="$t('page.userProfile.lastLoginAt')">
                    <NInput :value="profile?.lastLoginAt || '-'" disabled />
                  </NFormItem>
                  <NFormItem :label="$t('page.userProfile.createdAt')">
                    <NInput :value="profile?.createdAt" disabled />
                  </NFormItem>
                </NForm>

                <div class="flex justify-start ml-100px mt-4">
                  <NButton type="primary" @click="handleUpdateProfile">
                    <template #icon>
                      <SvgIcon icon="mdi:content-save-outline" />
                    </template>
                    {{ $t('common.save') }}
                  </NButton>
                </div>
              </div>
            </NTabPane>

            <NTabPane name="password" :tab="$t('page.userProfile.changePassword')">
              <div class="max-w-600px py-4">
                <NForm
                  ref="passwordFormRef"
                  :model="passwordForm"
                  :rules="passwordRules"
                  label-placement="left"
                  label-width="100"
                >
                  <NFormItem :label="$t('page.userProfile.oldPassword')" path="oldPassword">
                    <NInput
                      v-model:value="passwordForm.oldPassword"
                      type="password"
                      show-password-on="click"
                      :placeholder="$t('page.userProfile.oldPasswordPlaceholder')"
                    />
                  </NFormItem>
                  <NFormItem :label="$t('page.userProfile.newPassword')" path="newPassword">
                    <NInput
                      v-model:value="passwordForm.newPassword"
                      type="password"
                      show-password-on="click"
                      :placeholder="$t('page.userProfile.newPasswordPlaceholder')"
                    />
                  </NFormItem>
                  <NFormItem
                    :label="$t('page.userProfile.confirmPassword')"
                    path="confirmPassword"
                  >
                    <NInput
                      v-model:value="passwordForm.confirmPassword"
                      type="password"
                      show-password-on="click"
                      :placeholder="$t('page.userProfile.confirmPasswordPlaceholder')"
                    />
                  </NFormItem>
                </NForm>

                <div class="flex justify-start ml-100px mt-4">
                  <NButton type="primary" @click="handleChangePassword">
                    <template #icon>
                      <SvgIcon icon="mdi:lock-check-outline" />
                    </template>
                    {{ $t('page.userProfile.changePassword') }}
                  </NButton>
                </div>
              </div>
            </NTabPane>

            <NTabPane name="token" tab="Token 用量">
              <div class="py-4">
                <NGrid :x-gap="16" :y-gap="12" cols="2 s:4" responsive="screen">
                  <NGi>
                    <NCard size="small" :bordered="false" embedded>
                      <div class="text-sm text-gray-400">本月已用</div>
                      <div class="mt-1 text-2xl font-semibold">{{ fmtNum(tokenUsage?.month_used) }}</div>
                    </NCard>
                  </NGi>
                  <NGi>
                    <NCard size="small" :bordered="false" embedded>
                      <div class="text-sm text-gray-400">月度限额</div>
                      <div class="mt-1 text-2xl font-semibold">
                        <span v-if="!tokenUsage?.quota_month || tokenUsage.quota_month <= 0" class="text-gray-400">不限</span>
                        <template v-else>{{ fmtNum(tokenUsage.quota_month) }}</template>
                      </div>
                    </NCard>
                  </NGi>
                  <NGi>
                    <NCard size="small" :bordered="false" embedded>
                      <div class="text-sm text-gray-400">近 30 天用量</div>
                      <div class="mt-1 text-2xl font-semibold">
                        {{ fmtNum(tokenUsage?.trend?.reduce((acc, i) => acc + Number(i.total_tokens || 0), 0)) }}
                      </div>
                    </NCard>
                  </NGi>
                  <NGi>
                    <NCard size="small" :bordered="false" embedded>
                      <div class="text-sm text-gray-400">近 30 天调用</div>
                      <div class="mt-1 text-2xl font-semibold">
                        {{ fmtNum(tokenUsage?.trend?.reduce((acc, i) => acc + Number(i.calls || 0), 0)) }}
                      </div>
                    </NCard>
                  </NGi>
                </NGrid>

                <div v-if="quotaPercent !== null" class="mt-4">
                  <div class="mb-1 flex justify-between text-xs" :class="quotaPercent >= 100 ? 'text-red-500' : quotaPercent >= 80 ? 'text-orange-500' : 'text-gray-400'">
                    <span>本月额度使用</span>
                    <span>{{ quotaPercent }}%</span>
                  </div>
                  <NProgress
                    type="line"
                    :percentage="quotaPercent"
                    :status="quotaPercent >= 100 ? 'error' : quotaPercent >= 80 ? 'warning' : 'success'"
                    :show-indicator="false"
                  />
                </div>

                <NCard size="small" :bordered="false" embedded title="近 30 天用量趋势" class="mt-4">
                  <div ref="tokenChartRef" class="h-260px" />
                </NCard>

                <NCard size="small" :bordered="false" embedded title="近 30 天模型分布" class="mt-4">
                  <div v-if="tokenUsage?.by_model?.length" class="space-y-3">
                    <div v-for="m in tokenUsage.by_model" :key="m.model" class="flex items-center gap-3">
                      <span class="w-160px truncate text-sm" :title="m.model || '默认模型'">{{ m.model || '默认模型' }}</span>
                      <NProgress
                        type="line"
                        class="flex-1"
                        :percentage="Math.round((Number(m.total_tokens) / Math.max(...tokenUsage.by_model.map(x => Number(x.total_tokens), 1))) * 100)"
                        :show-indicator="false"
                      />
                      <span class="w-100px text-right text-xs text-gray-500">{{ fmtNum(m.total_tokens) }}</span>
                    </div>
                  </div>
                  <div v-else class="py-4 text-center text-sm text-gray-400">暂无用量数据</div>
                </NCard>
              </div>
            </NTabPane>

            <NTabPane name="theme" :tab="$t('page.userProfile.themeSettings')">
              <div class="py-4">
                <NTabs v-model:value="themeTab" type="segment" size="small" class="mb-16px">
                  <NTab name="appearance" :tab="$t('theme.tabs.appearance')"></NTab>
                  <NTab name="layout" :tab="$t('theme.tabs.layout')"></NTab>
                  <NTab name="general" :tab="$t('theme.tabs.general')"></NTab>
                  <NTab name="preset" :tab="$t('theme.tabs.preset')"></NTab>
                </NTabs>
                <div class="min-h-300px">
                  <KeepAlive>
                    <AppearanceSettings v-if="themeTab === 'appearance'" />
                    <LayoutSettings v-else-if="themeTab === 'layout'" />
                    <GeneralSettings v-else-if="themeTab === 'general'" />
                    <PresetSettings v-else-if="themeTab === 'preset'" />
                  </KeepAlive>
                </div>
                <NDivider />
                <ConfigOperation />
              </div>
            </NTabPane>

            <NTabPane name="notification" tab="通知渠道">
              <div class="max-w-600px py-4">
                <NCard title="通知渠道设置">
                  <NSpin :show="notificationLoading">
                    <div class="space-y-4">
                      <NAlert type="info">
                        选择接收通知的渠道。备忘提醒将发送到你启用的所有渠道。
                      </NAlert>

                      <NCheckboxGroup v-model:value="notificationChannels">
                        <NSpace vertical>
                          <NCheckbox value="email" label="邮件通知">
                            <div class="flex items-center gap-2">
                              <SvgIcon icon="mdi:email-outline" class="text-20px" />
                              <div>
                                <div class="font-medium">邮件通知</div>
                                <div class="text-12px text-gray-500">
                                  {{ profile?.email || '未设置邮箱' }}
                                </div>
                              </div>
                            </div>
                          </NCheckbox>

                          <NCheckbox
                            value="telegram"
                            :disabled="!telegramConfigured || !telegramStatus.isBound"
                            label="Telegram 通知"
                          >
                            <div class="flex items-center gap-2">
                              <SvgIcon icon="mdi:telegram" class="text-20px" />
                              <div>
                                <div class="font-medium">Telegram 通知</div>
                                <div class="text-12px text-gray-500">
                                  <template v-if="!telegramConfigured">
                                    系统未配置 Telegram Bot
                                  </template>
                                  <template v-else-if="!telegramStatus.isBound">
                                    请先绑定 Telegram 账号
                                  </template>
                                  <template v-else>
                                    @{{ telegramStatus.telegramUsername }}
                                  </template>
                                </div>
                              </div>
                            </div>
                          </NCheckbox>
                        </NSpace>
                      </NCheckboxGroup>

                      <div class="flex justify-start mt-4">
                        <NButton
                          type="primary"
                          :loading="savingNotification"
                          @click="handleSaveNotificationPreference"
                        >
                          <template #icon>
                            <SvgIcon icon="mdi:content-save-outline" />
                          </template>
                          保存设置
                        </NButton>
                      </div>
                    </div>
                  </NSpin>
                </NCard>
              </div>
            </NTabPane>

            <NTabPane v-if="telegramConfigured" name="telegram" :tab="$t('page.userProfile.telegramBinding')">
              <div class="max-w-600px py-4">
                <NCard :title="$t('page.userProfile.telegramBinding')">
                  <NSpin :show="telegramLoading">
                    <div v-if="telegramStatus.isBound" class="space-y-4">
                      <NAlert type="success" :title="$t('page.userProfile.telegramBound')">
                        <template #default>
                          {{ $t('page.userProfile.telegramUsername') }}: {{ telegramStatus.telegramUsername }}
                        </template>
                      </NAlert>
                      <NPopconfirm @positive-click="handleUnbindTelegram">
                        <template #trigger>
                          <NButton type="error">
                            <template #icon>
                              <SvgIcon icon="mdi:link-variant-off" />
                            </template>
                            {{ $t('page.userProfile.telegramUnbind') }}
                          </NButton>
                        </template>
                        {{ $t('page.userProfile.telegramUnbindConfirm') }}
                      </NPopconfirm>
                    </div>

                    <div v-else class="space-y-4">
                      <NAlert type="warning" :title="$t('page.userProfile.telegramNotBound')">
                        <template #default>
                          {{ $t('page.userProfile.telegramBindCodeHint') }}
                        </template>
                      </NAlert>

                      <div v-if="telegramBindCode" class="space-y-4">
                        <NCard embedded>
                          <div class="text-center">
                            <div class="text-32px font-bold tracking-wider mb-4 font-mono">
                              /bind {{ telegramBindCode.bindCode }}
                            </div>
                            <NText type="secondary">
                              {{ $t('page.userProfile.telegramBindCodeExpire') }}:
                              {{ formatExpireTime(telegramBindCode.expiresAt) }}
                            </NText>
                            <div v-if="telegramBindCode.botName" class="mt-2">
                              <NText type="secondary">
                                Bot: @{{ telegramBindCode.botName }}
                              </NText>
                            </div>
                          </div>
                        </NCard>

                        <div class="flex gap-2 justify-center">
                          <NButton type="primary" @click="handleCopyBindCode">
                            <template #icon>
                              <SvgIcon icon="mdi:content-copy" />
                            </template>
                            复制命令
                          </NButton>
                          <NButton :loading="telegramLoading" @click="handleGenerateBindCode">
                            <template #icon>
                              <SvgIcon icon="mdi:refresh" />
                            </template>
                            重新生成
                          </NButton>
                        </div>
                      </div>

                      <NButton v-else type="primary" :loading="telegramLoading" @click="handleGenerateBindCode">
                        <template #icon>
                          <SvgIcon icon="mdi:link-variant" />
                        </template>
                        {{ $t('page.userProfile.telegramGenerateCode') }}
                      </NButton>
                    </div>
                  </NSpin>
                </NCard>
              </div>
            </NTabPane>
          </NTabs>
        </NCard>
      </div>
    </NSpin>
  </div>
</template>

<style scoped></style>
