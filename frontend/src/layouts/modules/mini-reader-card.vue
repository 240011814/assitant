<script setup lang="ts">
import { computed } from 'vue';
import { useRouterPush } from '@/hooks/common/router';
import { useReaderSessionStore } from '@/store/modules/reader-session';
import { useThemeStore } from '@/store/modules/theme';
import { $t } from '@/locales';

/**
 * 全局阅读小窗卡片 (固定右下角, 挂载于 base-layout, 任意管理页可见):
 * 最小化的阅读会话 (RSS/分级阅读); 点卡片跳回对应页面恢复续读, × 丢弃。
 */
defineOptions({
  name: 'MiniReaderCard'
});

const { routerPushByKey } = useRouterPush();
const sessionStore = useReaderSessionStore();
const themeStore = useThemeStore();

// 固定 footer (absolute 覆盖视口底部, z-index 95) 会盖住卡片底部: 上移让出 footer 高度
const cardBottom = computed(() => {
  const footerOffset = themeStore.footer.visible && themeStore.footer.fixed ? themeStore.footer.height : 0;
  return `${footerOffset + 16}px`;
});

const icon = computed(() =>
  sessionStore.session?.kind === 'reading' ? 'mdi:book-open-variant' : 'mdi:rss'
);

function restore() {
  const session = sessionStore.session;
  if (!session) return;
  sessionStore.requestRestore();
  if (session.kind === 'rss') {
    routerPushByKey('ai_rss');
  } else {
    routerPushByKey('ai_reading-detail', { params: { id: String(session.itemId) } });
  }
}
</script>

<template>
  <Transition name="mini-fade">
    <div
      v-if="sessionStore.session && !sessionStore.restorePending"
      class="mini-reader fixed right-4 z-96 w-300px overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-[#18181c] cursor-pointer transition-shadow hover:shadow-[0_8px_30px_rgba(0,0,0,0.18)]"
      :style="{ bottom: cardBottom }"
      :title="$t('page.ai.reader.continueHint')"
      @click="restore"
    >
      <div class="flex items-center gap-2.5 px-3 py-2.5">
        <SvgIcon :icon="icon" class="text-primary text-20px shrink-0" />
        <div class="flex-1 min-w-0">
          <div class="text-13px font-medium text-gray-800 dark:text-gray-200 truncate">
            {{ sessionStore.session.title }}
          </div>
          <div class="text-11px text-gray-400 mt-0.5">
            {{ sessionStore.session.subtitle }} · {{ $t('page.ai.reader.progress', { progress: sessionStore.session.progress }) }}
          </div>
        </div>
        <NButton size="tiny" quaternary circle :title="$t('page.ai.reader.discardHint')" @click.stop="sessionStore.dismiss()">
          <template #icon>
            <SvgIcon icon="mdi:close" />
          </template>
        </NButton>
      </div>
      <!-- 阅读进度条 (最小化瞬间冻结) -->
      <div class="h-0.5 bg-gray-100 dark:bg-gray-800">
        <div class="h-full bg-primary transition-[width] duration-300" :style="{ width: sessionStore.session.progress + '%' }" />
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.mini-fade-enter-active,
.mini-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.mini-fade-enter-from,
.mini-fade-leave-to {
  opacity: 0;
  transform: translateY(12px);
}
</style>
