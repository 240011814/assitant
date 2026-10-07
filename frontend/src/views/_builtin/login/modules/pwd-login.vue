<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted } from "vue";
import { loginModuleRecord } from "@/constants/app";
import { useAuthStore } from "@/store/modules/auth";
import { useRouterPush } from "@/hooks/common/router";
import { useFormRules, useNaiveForm } from "@/hooks/common/form";
import { $t } from "@/locales";
import { fetchTwoFactorVerify, fetchRegisterStatus } from "@/service/api";

defineOptions({
  name: "PwdLogin",
});

const authStore = useAuthStore();
const { toggleLoginModule } = useRouterPush();
const { formRef, validate } = useNaiveForm();
const registerEnabled = ref(false);

// Local 2FA state - controls which UI to show
const show2FA = ref(false);

onMounted(async () => {
  try {
    const { data, error } = await fetchRegisterStatus();
    if (!error) {
      registerEnabled.value = data?.enabled ?? false;
    }
  } catch {
    registerEnabled.value = false;
  }
});

// Sync with store state
watch(
  () => authStore.twoFAState.need2FA,
  val => {
    show2FA.value = val;
  },
  { immediate: true }
);

interface FormModel {
  userName: string;
  password: string;
}

const model: FormModel = reactive({
  userName: "",
  password: "",
});

const rules = computed<Record<keyof FormModel, App.Global.FormRule[]>>(() => {
  // inside computed to make locale reactive, if not apply i18n, you can define it without computed
  const { formRules } = useFormRules();

  return {
    userName: formRules.userName,
    password: formRules.pwd,
  };
});

async function handleSubmit() {
  await validate();
  await authStore.login(model.userName, model.password);
}

// 2FA state (两步验证在个人中心自助绑定, 登录时仅需输入验证码)
const twoFAForm = reactive({
  code: ""
});
const twoFALoading = ref(false);
const twoFAError = ref("");

async function handleTwoFASubmit() {
  if (!twoFAForm.code || twoFAForm.code.length !== 6) {
    twoFAError.value = "请输入6位验证码";
    return;
  }
  twoFALoading.value = true;
  twoFAError.value = "";
  try {
    const tempToken = authStore.twoFAState.tempToken;
    const { data, error } = await fetchTwoFactorVerify(tempToken, twoFAForm.code);
    if (!error && data) {
      await authStore.completeTwoFactorLogin(data);
    } else {
      twoFAError.value = "验证码错误，请重试";
    }
  } catch {
    twoFAError.value = "验证失败，请重试";
  } finally {
    twoFALoading.value = false;
  }
}

function handleBackToLogin() {
  // Reset local state
  show2FA.value = false;
  twoFAForm.code = "";
  twoFAError.value = "";
  twoFALoading.value = false;
  // Reset store
  authStore.resetStore();
}
</script>

<template>
  <div>
    <!-- 2FA Verify UI -->
    <div v-if="show2FA" class="space-y-4">
      <div class="space-y-4">
        <div class="text-center">
          <div class="text-lg font-bold mb-2">身份验证</div>
          <div class="text-sm text-gray-500">请输入身份验证器应用中的6位验证码</div>
        </div>
        <NInput
          v-model:value="twoFAForm.code"
          placeholder="输入6位验证码"
          maxlength="6"
          @keyup.enter="handleTwoFASubmit"
        />
        <div v-if="twoFAError" class="text-red-500 text-sm text-center">
          {{ twoFAError }}
        </div>
        <NButton type="primary" size="large" round block :loading="twoFALoading" @click="handleTwoFASubmit">
          验证
        </NButton>
        <NButton block @click="handleBackToLogin"> 返回登录 </NButton>
      </div>
    </div>

    <!-- Normal login form -->
    <NForm
      v-else
      ref="formRef"
      :model="model"
      :rules="rules"
      size="large"
      :show-label="false"
      @keyup.enter="handleSubmit"
    >
      <NFormItem path="userName">
        <NInput
          v-model:value="model.userName"
          :placeholder="$t('page.login.common.userNamePlaceholder')"
        />
      </NFormItem>
      <NFormItem path="password">
        <NInput
          v-model:value="model.password"
          type="password"
          show-password-on="click"
          :placeholder="$t('page.login.common.passwordPlaceholder')"
        />
      </NFormItem>
      <NSpace vertical :size="24">
        <div class="flex-y-center justify-between">
          <NCheckbox>{{ $t("page.login.pwdLogin.rememberMe") }}</NCheckbox>
          <NButton quaternary @click="toggleLoginModule('reset-pwd')">
            {{ $t("page.login.pwdLogin.forgetPassword") }}
          </NButton>
        </div>
        <NButton
          type="primary"
          size="large"
          round
          block
          :loading="authStore.loginLoading"
          @click="handleSubmit"
        >
          {{ $t("common.confirm") }}
        </NButton>
        <NButton v-if="registerEnabled" block @click="toggleLoginModule('register')">
          {{ $t(loginModuleRecord.register) }}
        </NButton>
      </NSpace>
    </NForm>
  </div>
</template>

<style scoped></style>
