<script setup lang="ts">
import { computed, ref } from 'vue';
import { NButton, NInputNumber, NSelect, useMessage } from 'naive-ui';

const show = defineModel<boolean>('show', { default: false });

const props = defineProps<{
  /** 材料类型候选 (与零件表共用, 支持手动输入新类型) */
  typeOptions: Array<{ label: string; value: string }>;
}>();

const emit = defineEmits<{
  (e: 'apply', rows: Array<{ label: string; length: number; quantity: number }>): void;
}>();

const message = useMessage();

/** 切割件草稿: 名称 + 长度 + 单樘数量 */
interface PieceDraft {
  name: string;
  length: number;
  quantity: number;
}

/** 国标铝合金窗户类型: 切割件为参考公式 (45° 拼角外框不扣尺寸, 扇料按框料宽 c 扣减), 生成后可在清单中逐行调整 */
interface WindowType {
  value: string;
  label: string;
  /** 是否含扇料 (需要框料宽参数) */
  needsSash: boolean;
  build: (w: number, h: number, c: number) => PieceDraft[];
}

const windowTypes: WindowType[] = [
  {
    value: 'fixed',
    label: '固定窗',
    needsSash: false,
    build: (w, h) => [
      { name: '外框横梃', length: w, quantity: 2 },
      { name: '外框竖梃', length: h, quantity: 2 }
    ]
  },
  {
    value: 'casement',
    label: '平开窗 (单扇)',
    needsSash: true,
    build: (w, h, c) => [
      { name: '外框横梃', length: w, quantity: 2 },
      { name: '外框竖梃', length: h, quantity: 2 },
      { name: '扇横梃', length: w - 2 * c, quantity: 2 },
      { name: '扇竖梃', length: h - 2 * c, quantity: 2 }
    ]
  },
  {
    value: 'casement2',
    label: '平开窗 (双扇/对开)',
    needsSash: true,
    build: (w, h, c) => [
      { name: '外框横梃', length: w, quantity: 2 },
      { name: '外框竖梃', length: h, quantity: 2 },
      { name: '中梃', length: h - 2 * c, quantity: 1 },
      { name: '扇横梃', length: (w - 3 * c) / 2, quantity: 4 },
      { name: '扇竖梃', length: h - 2 * c, quantity: 4 }
    ]
  },
  {
    value: 'sliding',
    label: '推拉窗 (两扇)',
    needsSash: true,
    build: (w, h, c) => [
      { name: '外框上下横', length: w, quantity: 2 },
      { name: '外框边封', length: h, quantity: 2 },
      { name: '扇竖梃 (光企/勾企)', length: h - 2 * c, quantity: 4 },
      { name: '扇上下横', length: (w - 2 * c) / 2, quantity: 4 }
    ]
  }
];

const typeValue = ref(windowTypes[0]!.value);
const count = ref<number | null>(1);
const width = ref<number | null>(150);
const height = ref<number | null>(200);
const frame = ref<number | null>(5);
const materialType = ref<string | null>(null);

const currentType = computed(() => windowTypes.find(t => t.value === typeValue.value)!);

/** NSelect 选项 (纯 label/value, 过滤掉类型上的函数字段) */
const typeOptions = computed(() => windowTypes.map(t => ({ label: t.label, value: t.value })));

/** 单樘切割件预览 (尺寸/框料宽无效时为空) */
const previewPieces = computed<PieceDraft[]>(() => {
  const w = width.value;
  const h = height.value;
  const c = frame.value ?? 0;
  if (!w || !h || w <= 0 || h <= 0) return [];
  if (currentType.value.needsSash && (!c || c <= 0)) return [];
  return currentType.value.build(w, h, c);
});

function handleApply() {
  const w = width.value;
  const h = height.value;
  const c = frame.value ?? 0;
  const n = count.value ?? 1;
  if (!w || !h || w <= 0 || h <= 0) {
    message.error('请输入有效的窗户宽和高');
    return;
  }
  if (n < 1) {
    message.error('樘数至少为 1');
    return;
  }
  if (currentType.value.needsSash && (!c || c <= 0)) {
    message.error('请输入有效的框料宽');
    return;
  }
  for (const p of previewPieces.value) {
    if (p.length <= 0) {
      message.error(`切割件「${p.name}」长度非正, 请检查窗户尺寸与框料宽`);
      return;
    }
  }
  const label = materialType.value?.trim();
  if (!label) {
    message.error('请选择或输入材料类型');
    return;
  }
  const rows = previewPieces.value.map(p => ({
    label,
    length: Number(p.length.toFixed(2)),
    quantity: p.quantity * n
  }));
  emit('apply', rows);
  show.value = false;
}
</script>

<template>
  <NModal v-model:show="show" preset="card" title="国标窗户生成" class="w-560px">
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <span class="w-20">窗户类型</span>
        <NSelect v-model:value="typeValue" :options="typeOptions" class="flex-1" />
      </div>
      <div class="flex items-center gap-2">
        <span class="w-20">尺寸 (cm)</span>
        <span class="text-gray-400">宽</span>
        <NInputNumber v-model:value="width" :min="0" class="flex-1" />
        <span class="text-gray-400">×</span>
        <NInputNumber v-model:value="height" :min="0" class="flex-1" />
        <span class="text-gray-400">樘</span>
        <NInputNumber v-model:value="count" :min="1" class="w-24" />
      </div>
      <div v-if="currentType.needsSash" class="flex items-center gap-2">
        <span class="w-20">框料宽</span>
        <NInputNumber v-model:value="frame" :min="0" class="w-40" />
        <span class="text-gray-500 text-xs">cm, 扇料尺寸 = 窗洞尺寸 − 2×框料宽 (参考值, 生成后可修改)</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-20">材料类型</span>
        <NSelect
          v-model:value="materialType"
          :options="props.typeOptions"
          filterable
          tag
          clearable
          placeholder="选择或输入材料类型"
          class="flex-1"
        />
      </div>

      <!-- 单樘切割件预览 -->
      <div v-if="previewPieces.length > 0">
        <div class="mb-1 text-gray-500 text-xs">
          单樘切割件预览 (共 {{ previewPieces.reduce((s, p) => s + p.quantity, 0) }} 件,
          按樘数 {{ count ?? 1 }} 生成 {{ previewPieces.reduce((s, p) => s + p.quantity, 0) * (count ?? 1) }} 件)
        </div>
        <div class="flex flex-col gap-1 border border-gray-200 rounded-md p-2">
          <div v-for="p in previewPieces" :key="p.name" class="flex items-center justify-between text-sm">
            <span>{{ p.name }}</span>
            <span class="text-gray-500">{{ p.length }} cm × {{ p.quantity }}</span>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">取消</NButton>
        <NButton type="primary" @click="handleApply">生成并添加到清单</NButton>
      </div>
    </template>
  </NModal>
</template>
