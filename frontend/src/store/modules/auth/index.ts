import { computed, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import { defineStore } from 'pinia';
import { useLoading } from '@sa/hooks';
import { fetchGetUserInfo, fetchLogin, fetchRefreshToken } from '@/service/api';
import { useRouterPush } from '@/hooks/common/router';
import { localStg } from '@/utils/storage';
import { SetupStoreId } from '@/enum';
import { $t } from '@/locales';
import { useRouteStore } from '../route';
import { useTabStore } from '../tab';
import { useThemeStore } from '../theme';
import { clearAuthStorage, getToken } from './shared';

export const useAuthStore = defineStore(SetupStoreId.Auth, () => {
  const route = useRoute();
  const authStore = useAuthStore();
  const routeStore = useRouteStore();
  const tabStore = useTabStore();
  const { toLogin, redirectFromLogin } = useRouterPush(false);
  const { loading: loginLoading, startLoading, endLoading } = useLoading();

  const token = ref('');
  let refreshTimer: ReturnType<typeof setInterval> | null = null;

  const proxyMode = ref(false);

  const twoFAState = reactive({
    need2FA: false,
    tempToken: ''
  });

  const userInfo: Api.Auth.UserInfo = reactive({
    userId: '',
    userName: '',
    nickname: '',
    roles: [],
    buttons: [],
    permissions: []
  });

  /** is super role in static route */
  const isStaticSuper = computed(() => {
    const { VITE_AUTH_ROUTE_MODE, VITE_STATIC_SUPER_ROLE } = import.meta.env;

    return VITE_AUTH_ROUTE_MODE === 'static' && userInfo.roles.includes(VITE_STATIC_SUPER_ROLE);
  });

  function startRefreshTimer() {
    stopRefreshTimer();
    const interval = Number(import.meta.env.VITE_TOKEN_REFRESH_INTERVAL) || 10;
    refreshTimer = setInterval(async () => {
      const rToken = localStg.get('refreshToken');
      if (!rToken) return;
      const { data, error } = await fetchRefreshToken(rToken);
      if (!error) {
        localStg.set('token', data.token);
        localStg.set('refreshToken', data.refreshToken);
      }
    }, interval * 60 * 1000);
  }

  function stopRefreshTimer() {
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  }

  /** Is login */
  const isLogin = computed(() => Boolean(token.value));

  /** Reset auth store */
  async function resetStore() {
    recordUserId();
    stopRefreshTimer();

    clearAuthStorage();

    // Reset 2FA state
    twoFAState.need2FA = false;
    twoFAState.tempToken = '';

    proxyMode.value = false;

    authStore.$reset();

    if (!route.meta.constant) {
      await toLogin();
    }

    tabStore.cacheTabs();
    routeStore.resetStore();
  }

  /** Record the user ID of the previous login session Used to compare with the current user ID on next login */
  function recordUserId() {
    if (!userInfo.userId) {
      return;
    }

    // Store current user ID locally for next login comparison
    localStg.set('lastLoginUserId', userInfo.userId);
  }

  /**
   * Check if current login user is different from previous login user If different, clear all tabs
   *
   * @returns {boolean} Whether to clear all tabs
   */
  function checkTabClear(): boolean {
    if (!userInfo.userId) {
      return false;
    }

    const lastLoginUserId = localStg.get('lastLoginUserId');

    // Clear all tabs if current user is different from previous user
    if (!lastLoginUserId || lastLoginUserId !== userInfo.userId) {
      localStg.remove('globalTabs');
      tabStore.clearTabs();

      localStg.remove('lastLoginUserId');
      return true;
    }

    localStg.remove('lastLoginUserId');
    return false;
  }

  /**
   * Login
   *
   * @param userName User name
   * @param password Password
   * @param [redirect=true] Whether to redirect after login. Default is `true`
   */
  async function login(userName: string, password: string, redirect = true) {
    startLoading();

    const { data: loginResult, error } = await fetchLogin(userName, password);

    if (!error) {
      // Check if 2FA is required
      if (loginResult && 'need2fa' in loginResult && (loginResult as any).need2fa) {
        twoFAState.need2FA = true;
        twoFAState.tempToken = (loginResult as any).tempToken || '';
        localStg.set('temp2faToken', (loginResult as any).tempToken || '');
        endLoading();
        return;
      }

      // Normal login (no 2FA)
      const pass = await loginByToken(loginResult as Api.Auth.LoginToken);

      if (pass) {
        twoFAState.need2FA = false;
        // Check if the tab needs to be cleared
        const isClear = checkTabClear();
        let needRedirect = redirect;

        if (isClear) {
          // If the tab needs to be cleared,it means we don't need to redirect.
          needRedirect = false;
        }
        await redirectFromLogin(needRedirect);

        window.$notification?.success({
          title: $t('page.login.common.loginSuccess'),
          content: $t('page.login.common.welcomeBack', { userName: userInfo.userName }),
          duration: 4500
        });
      }
    } else {
      resetStore();
    }

    endLoading();
  }

  async function loginByToken(loginToken: Api.Auth.LoginToken) {
    // 1. stored in the localStorage, the later requests need it in headers
    localStg.set('token', loginToken.token);
    localStg.set('refreshToken', loginToken.refreshToken);

    // 2. get user info
    const pass = await getUserInfo();

    if (pass) {
      token.value = loginToken.token;
      startRefreshTimer();

      // 3. load theme settings from server
      const themeStore = useThemeStore();
      themeStore.loadFromServer();

      return true;
    }

    return false;
  }

  async function getUserInfo() {
    const { data: info, error } = await fetchGetUserInfo();

    if (!error) {
      // update store
      Object.assign(userInfo, info);

      return true;
    }

    return false;
  }

  async function initUserInfo() {
    const maybeToken = getToken();

    if (maybeToken) {
      token.value = maybeToken;
      const pass = await getUserInfo();

      if (pass) {
        startRefreshTimer();
        // load theme settings from server on page refresh
        const themeStore = useThemeStore();
        themeStore.loadFromServer();
      } else {
        resetStore();
      }
    }
  }

  async function completeTwoFactorLogin(loginToken: Api.Auth.LoginToken) {
    twoFAState.need2FA = false;
    twoFAState.tempToken = '';
    localStg.remove('temp2faToken');

    const pass = await loginByToken(loginToken);
    if (pass) {
      checkTabClear();
      await redirectFromLogin(true);
      window.$notification?.success({
        title: $t('page.login.common.loginSuccess'),
        content: $t('page.login.common.welcomeBack', { userName: userInfo.userName }),
        duration: 4500
      });
    }
  }

  return {
    token,
    userInfo,
    isStaticSuper,
    isLogin,
    loginLoading,
    twoFAState,
    proxyMode,
    resetStore,
    login,
    loginByToken,
    completeTwoFactorLogin,
    initUserInfo
  };
});
