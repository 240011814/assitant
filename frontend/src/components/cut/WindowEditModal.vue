<script setup lang="ts">
import { computed, h, ref, watch } from 'vue';
import { NButton, NInput, NInputNumber, NModal, NSelect, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { saveCutWindow } from '@/service/api';
import WindowGridPreview from './WindowGridPreview.vue';
import {
  CELL_FIXED,
  CELL_SASH,
  WINDOW_TEMPLATES,
  availableColWidth,
  availableRowHeight,
  buildPieces,
  checkGridGeometry,
  findWindowTemplate,
  round2,
  windowItemFromTemplate,
  type WindowItem
} from './window-template';

/**
 * 窗户单编辑器: 左侧分格预览 (点击格子切换 固定/开启) + 右侧参数, 配好的窗加入清单,
 * 一单可含多种类型的多樘窗, 整单保存入库。
 */
const show = defineModel<boolean>('show', { default: false });

const props = defineProps<{
  /** 材料类型候选 (与零件表共用) */
  typeOptions: Array<{ label: string; value: string }>;
  /** 待编辑的窗户单 (null = 新建) */
  order?: Api.Cut.CutWindow | null;
}>();

const emit = defineEmits<{ (e: 'saved'): void }>();

const message = useMessage();

const templateOptions = WINDOW_TEMPLATES.map(t => ({ label: t.label, value: t.key }));

// ===== 窗户单内容 =====
const name = ref('');
const windows = ref<WindowItem[]>([]);
const saving = ref(false);

// ===== 编辑区 (当前正在配置的一樘窗) =====
const curType = ref<string>(WINDOW_TEMPLATES[0]!.key);
const width = ref<number | null>(150);
const height = ref<number | null>(200);
const frameWidth = ref<number | null>(5);
const cols = ref<number[]>([140]);
const rows = ref<number[]>([190]);
const cells = ref<number[][]>([[CELL_FIXED]]);
const sliding = ref(false);
const materialType = ref<string | null>(null);
const count = ref<number | null>(1);
/** 正在回改清单中的下标, -1 = 新窗 */
const editingIndex = ref(-1);

const modalTitle = computed(() => (props.order ? $t('page.cut.wtEdit') : $t('page.cut.wtAdd')));

/** 均分并吸收舍入差 (总和精确等于 total) */
function evenSplit(total: number, n: number): number[] {
  const each = round2(total / n);
  const arr = Array.from({ length: n }, () => each);
  if (n > 0) {
    const diff = round2(total - each * n);
    arr[n - 1] = round2(arr[n - 1]! + diff);
  }
  return arr;
}

/** 按比例缩放到新的总和 (末位吸收舍入差, 总和精确) */
function rescale(arr: number[], total: number): number[] {
  const sum = arr.reduce((s, v) => s + v, 0);
  const out = sum > 0 ? arr.map(v => round2((v / sum) * total)) : evenSplit(total, arr.length);
  if (out.length > 0) {
    const diff = round2(total - out.reduce((s, v) => s + v, 0));
    out[out.length - 1] = round2(out[out.length - 1]! + diff);
  }
  return out;
}

function applyTemplate(key: string) {
  const def = findWindowTemplate(key);
  if (!def) return;
  curType.value = key;
  sliding.value = def.sliding;
  width.value = def.defaults.width;
  height.value = def.defaults.height;
  frameWidth.value = def.defaults.frameWidth;
  cols.value = evenSplit(availableColWidth(def.defaults.width, def.defaults.frameWidth, def.cols, def.sliding), def.cols);
  rows.value = evenSplit(availableRowHeight(def.defaults.height, def.defaults.frameWidth, def.rows), def.rows);
  cells.value = def.cells.map(r => [...r]);
  editingIndex.value = -1;
}

/** 总尺寸/框料宽变化: 等比缩放净宽高, 保持几何自洽 */
function onOuterChange() {
  const w = width.value;
  const h = height.value;
  const c = frameWidth.value;
  if (!w || !h || c === null || c < 0) return;
  cols.value = rescale(cols.value, availableColWidth(w, c, cols.value.length, sliding.value));
  rows.value = rescale(rows.value, availableRowHeight(h, c, rows.value.length));
}

function setColCount(n: number) {
  const w = width.value;
  const c = frameWidth.value;
  if (!w || c === null || c < 0 || n < 1 || n > 20) return;
  cols.value = evenSplit(availableColWidth(w, c, n, sliding.value), n);
  cells.value = cells.value.map(rowCells => {
    const next = [...rowCells];
    while (next.length < n) next.push(next[next.length - 1] ?? CELL_FIXED);
    return next.slice(0, n);
  });
}

function setRowCount(n: number) {
  const h = height.value;
  const c = frameWidth.value;
  if (!h || c === null || c < 0 || n < 1 || n > 20) return;
  rows.value = evenSplit(availableRowHeight(h, c, n), n);
  const next = cells.value.map(r => [...r]);
  while (next.length < n) next.push([...(next[next.length - 1] ?? Array.from({ length: cols.value.length }, () => CELL_FIXED))]);
  cells.value = next.slice(0, n);
}

/** 手改某列净宽: 差额按其余列现值比例分摊, 总和保持自洽 */
function setColWidth(index: number, v: number | null) {
  const w = width.value;
  const c = frameWidth.value;
  if (!w || c === null || c < 0 || v === null || v <= 0) return;
  const avail = availableColWidth(w, c, cols.value.length, sliding.value);
  const target = Math.min(v, round2(avail - 0.1 * (cols.value.length - 1)));
  const restSum = round2(avail - target);
  const rest = cols.value.filter((_, i) => i !== index);
  const oldRestSum = rest.reduce((s, cv) => s + cv, 0);
  const newRest = oldRestSum > 0 ? rest.map(cv => round2((cv / oldRestSum) * restSum)) : evenSplit(restSum, rest.length);
  if (newRest.length > 0) {
    const diff = round2(restSum - newRest.reduce((s, cv) => s + cv, 0));
    newRest[newRest.length - 1] = round2(newRest[newRest.length - 1]! + diff);
  }
  let ri = 0;
  cols.value = cols.value.map((_, i) => (i === index ? round2(target) : newRest[ri++]!));
}

function setRowHeight(index: number, v: number | null) {
  const h = height.value;
  const c = frameWidth.value;
  if (!h || c === null || c < 0 || v === null || v <= 0) return;
  const avail = availableRowHeight(h, c, rows.value.length);
  const target = Math.min(v, round2(avail - 0.1 * (rows.value.length - 1)));
  const restSum = round2(avail - target);
  const rest = rows.value.filter((_, i) => i !== index);
  const oldRestSum = rest.reduce((s, cv) => s + cv, 0);
  const newRest = oldRestSum > 0 ? rest.map(cv => round2((cv / oldRestSum) * restSum)) : evenSplit(restSum, rest.length);
  if (newRest.length > 0) {
    const diff = round2(restSum - newRest.reduce((s, cv) => s + cv, 0));
    newRest[newRest.length - 1] = round2(newRest[newRest.length - 1]! + diff);
  }
  let ri = 0;
  rows.value = rows.value.map((_, i) => (i === index ? round2(target) : newRest[ri++]!));
}

/** 编辑区当前窗 (尺寸不全时给最小占位, 供校验与预览兜底) */
const currentItem = computed<WindowItem>(() => {
  const w = width.value ?? 0;
  const h = height.value ?? 0;
  const c = frameWidth.value ?? 0;
  return {
    type: curType.value,
    width: w,
    height: h,
    frameWidth: c,
    materialType: materialType.value?.trim() ?? '',
    count: Math.max(count.value ?? 1, 1),
    grid: {
      cols: cols.value,
      rows: rows.value,
      cells: cells.value,
      sliding: sliding.value
    }
  };
});

const geometryError = computed(() => checkGridGeometry(currentItem.value));

const previewPieces = computed(() => buildPieces(currentItem.value));
const previewTotal = computed(() => previewPieces.value.reduce((s, p) => s + p.quantity, 0));

function toggleCell(row: number, col: number) {
  const line = cells.value[row];
  if (!line) return;
  line[col] = line[col] === CELL_SASH ? CELL_FIXED : CELL_SASH;
}

/** 把清单中第 index 窗载回编辑区 */
function editItem(index: number) {
  const item = windows.value[index];
  if (!item) return;
  editingIndex.value = index;
  curType.value = item.type;
  sliding.value = item.grid.sliding;
  width.value = item.width;
  height.value = item.height;
  frameWidth.value = item.frameWidth;
  cols.value = [...item.grid.cols];
  rows.value = [...item.grid.rows];
  cells.value = item.grid.cells.map(r => [...r]);
  materialType.value = item.materialType;
  count.value = item.count;
}

function removeItem(index: number) {
  windows.value.splice(index, 1);
  if (editingIndex.value === index) editingIndex.value = -1;
  else if (editingIndex.value > index) editingIndex.value -= 1;
}

/** 加入/更新清单 (更新时替换原下标, 然后编辑区重置为新窗) */
function addToList() {
  const err = geometryError.value;
  if (err) {
    message.error(err);
    return;
  }
  if (!currentItem.value.materialType) {
    message.error($t('page.cut.wtNeedMaterial'));
    return;
  }
  const copy = JSON.parse(JSON.stringify(currentItem.value)) as WindowItem;
  if (editingIndex.value >= 0) {
    windows.value.splice(editingIndex.value, 1, copy);
  } else {
    windows.value.push(copy);
  }
  editingIndex.value = -1;
  // 重置编辑区为新窗 (保留材料类型与樘数, 方便连续录入同材料)
  applyTemplate(curType.value);
}

const listColumns = computed<DataTableColumns<WindowItem>>(() => [
  {
    title: $t('page.cut.wtTemplate'),
    key: 'type',
    width: 150,
    render: row => {
      const def = findWindowTemplate(row.type);
      return h('span', null, def?.label ?? row.type);
    }
  },
  { title: '尺寸(cm)', key: 'size', width: 110, render: row => `${row.width}×${row.height}` },
  {
    title: $t('page.cut.materialType'),
    key: 'materialType',
    width: 160,
    render: row =>
      h(NSelect, {
        value: row.materialType,
        options: props.typeOptions,
        filterable: true,
        tag: true,
        size: 'small',
        onUpdateValue: (v: string | null) => {
          row.materialType = v ?? '';
        }
      })
  },
  {
    title: $t('page.cut.wtCount'),
    key: 'count',
    width: 100,
    render: row =>
      h(NInputNumber, {
        value: row.count,
        min: 1,
        size: 'small',
        showButton: false,
        class: 'w-20',
        onUpdateValue: (v: number | null) => {
          row.count = Math.max(v ?? 1, 1);
        }
      })
  },
  {
    title: $t('page.cut.wtGrid'),
    key: 'grid',
    width: 130,
    render: row =>
      h('span', null, [
        `${row.grid.cols.length}列×${row.grid.rows.length}行`,
        row.grid.sliding ? h(NTag, { size: 'small', bordered: false, type: 'info', class: 'ml-1' }, { default: () => $t('page.cut.wtSliding') }) : null
      ])
  },
  {
    title: '操作',
    key: 'actions',
    width: 110,
    render(row, index) {
      return h('div', { class: 'flex gap-1' }, [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => editItem(index) },
          { default: () => '编辑' }
        ),
        h(
          NButton,
          { size: 'small', type: 'error', quaternary: true, onClick: () => removeItem(index) },
          { default: () => $t('common.delete') }
        )
      ]);
    }
  }
]);

async function handleSave() {
  if (editingIndex.value >= 0) {
    message.warning($t('page.cut.wtPendingEdit'));
    return;
  }
  if (windows.value.length === 0) {
    message.error($t('page.cut.wtEmptyOrder'));
    return;
  }
  for (const item of windows.value) {
    const err = checkGridGeometry(item);
    if (err) {
      message.error(`${findWindowTemplate(item.type)?.label ?? item.type}: ${err}`);
      return;
    }
    if (!item.materialType?.trim()) {
      message.error($t('page.cut.wtNeedMaterial'));
      return;
    }
    if (item.count < 1) item.count = 1;
  }
  saving.value = true;
  try {
    const { error } = await saveCutWindow({
      id: props.order?.id,
      name: name.value.trim() || undefined,
      spec: { windows: windows.value }
    });
    if (error) return;
    message.success($t('page.cut.wtSaved'));
    emit('saved');
    show.value = false;
  } finally {
    saving.value = false;
  }
}

// 打开时初始化: 编辑模式载入单据内容, 新建模式清空
watch(show, opened => {
  if (!opened) return;
  editingIndex.value = -1;
  saving.value = false;
  let list: WindowItem[] = [];
  if (props.order) {
    name.value = props.order.name ?? '';
    try {
      const spec = JSON.parse(props.order.spec) as Api.Cut.WindowSpec;
      if (spec && Array.isArray(spec.windows)) list = spec.windows;
    } catch {
      list = [];
    }
  } else {
    name.value = '';
  }
  windows.value = list.map(item => JSON.parse(JSON.stringify(item)) as WindowItem);
  materialType.value = windows.value[0]?.materialType ?? null;
  count.value = windows.value[0]?.count ?? 1;
  applyTemplate(WINDOW_TEMPLATES[0]!.key);
});
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="modalTitle" class="w-1000px max-w-[96vw]">
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <span class="w-20 shrink-0">{{ $t('page.cut.wtName') }}</span>
        <NInput v-model:value="name" :placeholder="$t('page.cut.wtNamePh')" />
      </div>

      <!-- 编辑区: 左预览 + 右参数 -->
      <div class="flex gap-4">
        <div class="w-360px shrink-0">
          <div class="border border-gray-200 rounded-md p-2">
            <WindowGridPreview :item="currentItem" :max-height="280" @toggle-cell="toggleCell" />
          </div>
          <div class="mt-1 text-center text-gray-400 text-xs">{{ $t('page.cut.wtPreviewTip') }}</div>
        </div>

        <div class="flex flex-1 flex-col gap-2">
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.wtTemplate') }}</span>
            <NSelect :value="curType" :options="templateOptions" @update:value="applyTemplate" />
          </div>
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.wtWidthLabel') }}</span>
            <NInputNumber :value="width" :min="1" class="flex-1" @update:value="onOuterChange" />
            <span class="text-gray-400">×</span>
            <NInputNumber :value="height" :min="1" class="flex-1" @update:value="onOuterChange" />
            <span class="w-20 shrink-0 text-right">{{ $t('page.cut.wtFrameWidth') }}</span>
            <NInputNumber :value="frameWidth" :min="0" class="w-30" @update:value="onOuterChange" />
          </div>
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.wtCols') }}</span>
            <NInputNumber
              :value="cols.length"
              :min="1"
              :max="20"
              class="w-24"
              @update:value="(v: number | null) => setColCount(v ?? 1)"
            />
            <span class="w-16 shrink-0 text-right">{{ $t('page.cut.wtRows') }}</span>
            <NInputNumber
              :value="rows.length"
              :min="1"
              :max="20"
              class="w-24"
              @update:value="(v: number | null) => setRowCount(v ?? 1)"
            />
            <span class="w-16 shrink-0 text-right">{{ $t('page.cut.wtCount') }}</span>
            <NInputNumber v-model:value="count" :min="1" class="w-24" />
          </div>
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.wtColWidths') }}</span>
            <div class="flex flex-wrap gap-1">
              <NInputNumber
                v-for="(cw, i) in cols"
                :key="`c${i}`"
                :value="cw"
                size="small"
                :min="0.1"
                class="w-24"
                @update:value="(v: number | null) => setColWidth(i, v)"
              />
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.wtRowHeights') }}</span>
            <div class="flex flex-wrap gap-1">
              <NInputNumber
                v-for="(rh, i) in rows"
                :key="`r${i}`"
                :value="rh"
                size="small"
                :min="0.1"
                class="w-24"
                @update:value="(v: number | null) => setRowHeight(i, v)"
              />
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span class="w-20 shrink-0">{{ $t('page.cut.materialType') }}</span>
            <NSelect
              v-model:value="materialType"
              :options="typeOptions"
              filterable
              tag
              clearable
              :placeholder="$t('page.cut.materialType')"
            />
          </div>

          <div v-if="geometryError" class="text-red-500 text-xs">{{ geometryError }}</div>

          <div class="flex items-start gap-2">
            <NButton type="primary" @click="addToList">
              {{ editingIndex >= 0 ? $t('page.cut.wtUpdateToList') : $t('page.cut.wtAddToList') }}
            </NButton>
            <div class="flex-1 rounded-md border border-gray-200 p-2">
              <div class="mb-1 text-gray-500 text-xs">
                {{ $t('page.cut.wtPieces') }} (共 {{ previewTotal }} 件)
              </div>
              <div class="flex max-h-32 flex-col gap-0.5 overflow-y-auto">
                <div v-for="p in previewPieces" :key="`${p.name}|${p.length}`" class="flex items-center justify-between text-sm">
                  <span>{{ p.name }}</span>
                  <span class="text-gray-500">{{ p.length }} cm × {{ p.quantity }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 窗户清单 -->
      <div>
        <div class="mb-1 font-bold">{{ $t('page.cut.wtListTitle') }} ({{ windows.length }})</div>
        <NDataTable :columns="listColumns" :data="windows" size="small" :bordered="false" />
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" @click="handleSave">{{ $t('page.cut.wtSave') }}</NButton>
      </div>
    </template>
  </NModal>
</template>
