<script setup lang="ts">
import { computed } from "vue";
import { NModal } from "naive-ui";
import { resolveRarity } from "../composables/rarity-theme";
import PrizeResultCard from "./prize-result-card.vue";

/** 转盘模式的抽奖结果弹窗: 共享 PrizeResultCard (与抽卡结果卡同一套视觉) */
const props = defineProps<{
  drawResult: Api.Lottery.DrawResult | null;
}>();

const show = defineModel<boolean>("show", { required: true });

const rarity = computed(() => {
  if (!props.drawResult?.isWinner || !props.drawResult.prize) return 1;
  return resolveRarity(props.drawResult.prize.prizeLevel);
});
</script>

<template>
  <NModal
    v-model:show="show"
    style="width: 90%; max-width: 400px"
    :bordered="false"
    :mask-closable="false"
  >
    <PrizeResultCard
      v-if="drawResult"
      :draw-result="drawResult"
      :rarity="rarity"
      variant="wheel"
      confirm-text="知道了"
      @confirm="show = false"
    />
  </NModal>
</template>
