import type { RouteMeta } from 'vue-router';
import ElegantVueRouter from '@elegant-router/vue/vite';
import type { RouteKey } from '@elegant-router/types';

// 手写路由 meta 的唯一维护点: routes.ts 里已有条目的 meta 在全量再生成时会保留,
// 但增量再生成只保留 title/i18nKey; 新增路由时在 customRouteMeta 补 icon/permissions/order 等,
// 再生成的 routes.ts 才能带上完整 meta (勿再把定制写进 route.json, 插件不读取它)
const customRouteMeta: Partial<Record<RouteKey, Partial<RouteMeta>>> = {
  '403': { hideInMenu: true },
  '404': { hideInMenu: true },
  '500': { hideInMenu: true },
  ai: { icon: 'mdi:robot-outline', order: 2 },
  ai_course: { icon: 'mdi:book-education', permissions: ['ai:course:view'], order: 5, keepAlive: true },
  'ai_course-detail': { permissions: ['ai:course:view'], hideInMenu: true },
  'ai_custom-training': { permissions: ['ai:custom-training:view'], hideInMenu: true, keepAlive: true },
  'ai_error-book': { icon: 'mdi:book-alert-outline', permissions: ['ai:error-book:view'], order: 7 },
  ai_exercise: { permissions: ['ai:chat:view'], hideInMenu: true, keepAlive: true },
  ai_history: { icon: 'mdi:history', permissions: ['ai:history:view'], order: 8 },
  ai_note: { icon: 'mdi:notebook-outline', permissions: ['ai:note:view'], order: 7 },
  ai_orchestration: { hideInMenu: true, keepAlive: true },
  ai_training: { icon: 'mdi:school-outline', order: 1, keepAlive: true },
  ai_vocabulary: { icon: 'mdi:book-open-outline', permissions: ['ai:vocabulary:view'], order: 6 },
  cut: { icon: 'mdi:scissors-cutting', order: 4, permissions: ['cut:menu:view'] },
  cut_bar: { icon: 'mdi:angle-acute', order: 1, permissions: ['cut:bar:compute'], keepAlive: true },
  'cut_bar-detail': { hideInMenu: true, permissions: ['cut:bar:compute'] },
  cut_history: { icon: 'mdi:history', order: 4, permissions: ['cut:record:view'] },
  cut_inventory: { icon: 'mdi:package-variant-closed', order: 3, permissions: ['cut:record:view'] },
  cut_plane: { icon: 'mdi:grid-large', order: 2, permissions: ['cut:plane:compute'], keepAlive: true },
  'cut_plane-detail': { hideInMenu: true, permissions: ['cut:plane:compute'] },
  'cut_window-template': { icon: 'mdi:window-maximize', order: 5, permissions: ['cut:record:view'] },
  home: { icon: 'mdi:monitor-dashboard', order: 1 },
  'iframe-page': { hideInMenu: true },
  login: { hideInMenu: true },
  lottery: { hideInMenu: true },
  share: { hideInMenu: true },
  system: { icon: 'mdi:cog-outline', order: 5, permissions: ['sys:menu:view'] },
  'system_agent-studio': { icon: 'mdi:sitemap-outline', permissions: ['system:orchestration:view'], order: 9 },
  'system_ai-config': {
    permissions: ['system:ai-provider:view', 'system:ai-model:view', 'system:ai-tool:view'],
    icon: 'mdi:robot-confused-outline'
  },
  'system_audit-log': { icon: 'mdi:shield-check-outline', permissions: ['system:audit:view'], order: 10 },
  system_config: { icon: 'mdi:tune-variant', permissions: ['R_SUPER'] },
  system_dashboard: { icon: 'mdi:view-dashboard-outline', permissions: ['system:dashboard:view'], order: 1 },
  system_job: { icon: 'mdi:clock-outline', permissions: ['job:manage'], order: 7 },
  system_lottery: { icon: 'mdi:gift', permissions: ['lottery:activity:view'] },
  'system_model-scenario': { icon: 'mdi:lightbulb-on-outline', permissions: ['model_scenario:view'], order: 6, keepAlive: true },
  system_permission: { icon: 'mdi:shield-key-outline', permissions: ['system:permission:view'] },
  system_skill: { icon: 'mdi:puzzle-outline', permissions: ['system:skill:view'], order: 8 },
  system_mcp: { icon: 'mdi:server-network-outline', permissions: ['system:mcp:manage'], order: 9 },
  system_user: { icon: 'mdi:account-cog-outline', permissions: ['system:user:list'] },
  tool: { icon: 'mdi:tools', order: 3 },
  tool_backtest: { icon: 'mdi:chart-bar', permissions: ['stock:screen:view'], order: 13 },
  tool_calendar: { icon: 'mdi:calendar-month-outline', order: 15 },
  tool_macro: { icon: 'mdi:bank-outline', permissions: ['stock:macro:view'], order: 14 },
  'tool_stock-alert': { icon: 'mdi:bell-outline', permissions: ['stock:watchlist:view'], order: 12 },
  tool_stockdetail: { permissions: ['stock:menu:view'], hideInMenu: true, activeMenu: 'tool_stockscreen', order: 16 },
  tool_stockscreen: { icon: 'mdi:chart-line', permissions: ['stock:screen:view'], order: 11 },
  tool_watchlist: { icon: 'mdi:star-outline', permissions: ['stock:watchlist:view'], order: 10 },
  user: { icon: 'mdi:account-outline', order: 99 },
  user_portrait: { icon: 'mdi:account-details-outline' },
  user_document: { icon: 'mdi:file-document-multiple-outline', permissions: ['document:view'] },
  user_profile: { icon: 'mdi:account-circle-outline' }
};

export function setupElegantRouter() {
  return ElegantVueRouter({
    layouts: {
      base: 'src/layouts/base-layout/index.vue',
      blank: 'src/layouts/blank-layout/index.vue'
    },
    routePathTransformer(routeName, routePath) {
      const key = routeName as RouteKey;

      if (key === 'login') {
        const modules: UnionKey.LoginModule[] = ['pwd-login', 'code-login', 'register', 'reset-pwd', 'bind-wechat'];

        const moduleReg = modules.join('|');

        return `/login/:module(${moduleReg})?`;
      }

      return routePath;
    },
    onRouteMetaGen(routeName) {
      const key = routeName as RouteKey;

      const constantRoutes: RouteKey[] = ['login', '403', '404', '500'];

      const meta: Partial<RouteMeta> = {
        title: key,
        i18nKey: `route.${key}` as App.I18n.I18nKey
      };

      if (constantRoutes.includes(key)) {
        meta.constant = true;
      }

      return { ...meta, ...customRouteMeta[key] };
    }
  });
}
