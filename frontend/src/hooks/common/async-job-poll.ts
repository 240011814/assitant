import { onBeforeUnmount, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { fetchGetAsyncJob } from '@/service/api';
import { $t } from '@/locales';

/**
 * 通用异步任务轮询 (Cloudflare 对源站有 100s 上限, 慢 LLM 操作一律异步 + 轮询):
 * 2.5s 一次查询 job 状态, running 更新进度, 终态收口。
 * - done: 回调 onDone (刷新详情/高亮等)
 * - failed: 弹 failTitle + 错误信息 (failTitle 支持函数, 每次失败时求值以跟随语言切换)
 * - 查询出错: 静默停止 (与原各页轮询一致)
 * 组件卸载自动停止; 切换目标 (如换文章) 时调用 stop(), 防止过期回调作用于新目标。
 */
export function useAsyncJobPolling(options: {
  failTitle: string | (() => string);
  onDone: () => Promise<void> | void;
}) {
  const message = useMessage();
  const polling = ref(false);
  const progress = ref('');
  let timer: number | null = null;

  function stop() {
    if (timer !== null) {
      window.clearInterval(timer);
      timer = null;
    }
    polling.value = false;
    progress.value = '';
  }

  function start(jobId: number) {
    stop();
    polling.value = true;
    timer = window.setInterval(async () => {
      const { data: job, error } = await fetchGetAsyncJob(jobId);
      if (error || !job) {
        stop();
        return;
      }
      if (job.status === 'running') {
        progress.value = job.progress || '';
        return;
      }
      stop();
      if (job.status === 'failed') {
        const failLabel = typeof options.failTitle === 'function' ? options.failTitle() : options.failTitle;
        message.error(`${failLabel}: ${job.error || $t('page.ai.reader.unknownError')}`);
        return;
      }
      await options.onDone();
    }, 2500);
  }

  onBeforeUnmount(stop);

  return { polling, progress, start, stop };
}
