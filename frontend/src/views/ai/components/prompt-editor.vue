<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import {
  useMessage,
  NPopconfirm,
  NTabs,
  NTabPane,
  NTag,
  NButton,
  NInput,
  NScrollbar,
  NModal,
  NBadge,
  NEmpty,
} from "naive-ui";
import {
  fetchGetUserPrompt,
  fetchSaveUserPrompt,
  fetchResetUserPrompt,
  fetchSwitchUserPrompt,
  fetchDeleteUserPromptVersion,
} from "@/service/api";
import { format } from "date-fns";

 
const props = defineProps<{
  agentId: number;
  defaultPrompt: string;
}>();

const emit = defineEmits<{
  updated: [];
}>();

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const promptData = ref({
  effective_prompt: "",
  default_prompt: props.defaultPrompt,
  versions: [] as any[],
  is_customized: false,
});

const editingPrompt = ref("");
const remark = ref("");
const showDefault = ref(false);
const activeTab = ref("editor"); // 'editor' | 'history'

// 对比相关
const showCompareModal = ref(false);
const compareTarget = ref<any>(null);

type DiffLineType = "equal" | "added" | "removed" | "changed" | "empty";

interface DiffLine {
  content: string;
  lineNumber?: number;
  segments: DiffSegment[];
  type: DiffLineType;
}

interface DiffSegment {
  content: string;
  changed: boolean;
}

interface DiffRow {
  left: DiffLine;
  right: DiffLine;
}

interface DiffOperation {
  content: string;
  lineNumber: number;
  side: "left" | "right" | "both";
  type: "equal" | "added" | "removed";
}

async function loadPrompt() {
  loading.value = true;
  try {
    const { data } = await fetchGetUserPrompt(props.agentId);
    if (data) {
      promptData.value = {
        ...data,
        default_prompt: data.default_prompt || props.defaultPrompt,
      };
      // 只有当编辑器内容为空或者与之前同步时才更新编辑器
      if (!editingPrompt.value || activeTab.value === "history") {
        editingPrompt.value =
          data.effective_prompt || data.default_prompt || props.defaultPrompt;
      }
    }
  } catch (err: any) {
    message.error(`加载失败: ${err?.message || "未知错误"}`);
  } finally {
    loading.value = false;
  }
}

async function handleSave() {
  if (!editingPrompt.value.trim()) {
    message.warning("提示词不能为空");
    return;
  }
  saving.value = true;
  try {
    await fetchSaveUserPrompt(props.agentId, editingPrompt.value, remark.value);
    message.success("新版本已保存并启用");
    remark.value = "";
    await loadPrompt();
    emit("updated");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    saving.value = false;
  }
}

async function handleSwitch(versionId: number) {
  loading.value = true;
  try {
    await fetchSwitchUserPrompt(props.agentId, versionId);
    message.success("已切换版本");
    await loadPrompt();
    emit("updated");
  } catch (err: any) {
    message.error(`切换失败: ${err?.message || "未知错误"}`);
  } finally {
    loading.value = false;
  }
}

async function handleDelete(versionId: number) {
  try {
    await fetchDeleteUserPromptVersion(props.agentId, versionId);
    message.success("版本已删除");
    await loadPrompt();
    emit("updated");
  } catch (err: any) {
    message.error(`删除失败: ${err?.message || "未知错误"}`);
  }
}

async function handleReset() {
  saving.value = true;
  try {
    await fetchResetUserPrompt(props.agentId);
    message.success("已恢复系统默认设置");
    editingPrompt.value = "";
    await loadPrompt();
    emit("updated");
  } catch (err: any) {
    message.error(`重置失败: ${err?.message || "未知错误"}`);
  } finally {
    saving.value = false;
  }
}

function viewHistory(v: any) {
  editingPrompt.value = v.custom_prompt;
  activeTab.value = "editor";
  message.info(`已加载版本 v${v.version} 到编辑器`);
}

function openCompare(v: any) {
  compareTarget.value = v;
  showCompareModal.value = true;
}

// 同步滚动逻辑
const leftScrollRef = ref<HTMLElement | null>(null);
const rightScrollRef = ref<HTMLElement | null>(null);

function syncScroll(event: Event, side: "left" | "right") {
  const source = event.target as HTMLElement;
  const target = side === "left" ? rightScrollRef.value : leftScrollRef.value;

  if (target) {
    target.scrollTop = source.scrollTop;
  }
}

onMounted(() => {
  loadPrompt();
});

const hasChanges = computed(() => {
  return (
    editingPrompt.value !== promptData.value.effective_prompt
  );
});

const activeVersion = computed(() => {
  return promptData.value.versions.find((v) => v.is_active);
});

const currentPrompt = computed(() => {
  return promptData.value.effective_prompt || promptData.value.default_prompt;
});

const compareDiffRows = computed(() => {
  return buildDiffRows(compareTarget.value?.custom_prompt || "", currentPrompt.value);
});

function splitLines(value: string) {
  return value.length ? value.split(/\r?\n/) : [""];
}

function buildDiffOperations(leftText: string, rightText: string) {
  const leftLines = splitLines(leftText);
  const rightLines = splitLines(rightText);
  const rowCount = leftLines.length;
  const columnCount = rightLines.length;
  const dp = Array.from({ length: rowCount + 1 }, () => Array(columnCount + 1).fill(0));

  for (let row = rowCount - 1; row >= 0; row -= 1) {
    for (let column = columnCount - 1; column >= 0; column -= 1) {
      dp[row][column] =
        leftLines[row] === rightLines[column]
          ? dp[row + 1][column + 1] + 1
          : Math.max(dp[row + 1][column], dp[row][column + 1]);
    }
  }

  const operations: DiffOperation[] = [];
  let leftIndex = 0;
  let rightIndex = 0;

  while (leftIndex < rowCount && rightIndex < columnCount) {
    if (leftLines[leftIndex] === rightLines[rightIndex]) {
      operations.push({
        content: leftLines[leftIndex],
        lineNumber: leftIndex + 1,
        side: "both",
        type: "equal",
      });
      leftIndex += 1;
      rightIndex += 1;
    } else if (dp[leftIndex + 1][rightIndex] >= dp[leftIndex][rightIndex + 1]) {
      operations.push({
        content: leftLines[leftIndex],
        lineNumber: leftIndex + 1,
        side: "left",
        type: "removed",
      });
      leftIndex += 1;
    } else {
      operations.push({
        content: rightLines[rightIndex],
        lineNumber: rightIndex + 1,
        side: "right",
        type: "added",
      });
      rightIndex += 1;
    }
  }

  while (leftIndex < rowCount) {
    operations.push({
      content: leftLines[leftIndex],
      lineNumber: leftIndex + 1,
      side: "left",
      type: "removed",
    });
    leftIndex += 1;
  }

  while (rightIndex < columnCount) {
    operations.push({
      content: rightLines[rightIndex],
      lineNumber: rightIndex + 1,
      side: "right",
      type: "added",
    });
    rightIndex += 1;
  }

  return operations;
}

function buildDiffRows(leftText: string, rightText: string): DiffRow[] {
  const operations = buildDiffOperations(leftText, rightText);
  const rows: DiffRow[] = [];
  let index = 0;

  while (index < operations.length) {
    const operation = operations[index];

    if (operation.type === "equal") {
      rows.push({
        left: createDiffLine(operation.content, "equal", operation.lineNumber),
        right: createDiffLine(operation.content, "equal", operation.lineNumber),
      });
      index += 1;
      continue;
    }

    const removed: DiffOperation[] = [];
    const added: DiffOperation[] = [];

    while (index < operations.length && operations[index].type !== "equal") {
      if (operations[index].type === "removed") {
        removed.push(operations[index]);
      } else {
        added.push(operations[index]);
      }
      index += 1;
    }

    const maxLength = Math.max(removed.length, added.length);

    for (let rowIndex = 0; rowIndex < maxLength; rowIndex += 1) {
      const removedLine = removed[rowIndex];
      const addedLine = added[rowIndex];
      const isChanged = Boolean(removedLine && addedLine);
      const inlineDiff = isChanged
        ? buildInlineDiffSegments(removedLine.content, addedLine.content)
        : null;

      rows.push({
        left: removedLine
          ? createDiffLine(
              removedLine.content,
              isChanged ? "changed" : "removed",
              removedLine.lineNumber,
              inlineDiff?.left
            )
          : createDiffLine("", "empty"),
        right: addedLine
          ? createDiffLine(
              addedLine.content,
              isChanged ? "changed" : "added",
              addedLine.lineNumber,
              inlineDiff?.right
            )
          : createDiffLine("", "empty"),
      });
    }
  }

  return rows;
}

function createDiffLine(
  content: string,
  type: DiffLineType,
  lineNumber?: number,
  segments?: DiffSegment[]
): DiffLine {
  return {
    content,
    lineNumber,
    segments: segments || [{ content, changed: false }],
    type,
  };
}

function buildInlineDiffSegments(leftContent: string, rightContent: string) {
  const leftChars = Array.from(leftContent);
  const rightChars = Array.from(rightContent);

  if (leftChars.length * rightChars.length > 160000) {
    return {
      left: [{ content: leftContent, changed: true }],
      right: [{ content: rightContent, changed: true }],
    };
  }

  const rowCount = leftChars.length;
  const columnCount = rightChars.length;
  const dp = Array.from({ length: rowCount + 1 }, () => Array(columnCount + 1).fill(0));

  for (let row = rowCount - 1; row >= 0; row -= 1) {
    for (let column = columnCount - 1; column >= 0; column -= 1) {
      dp[row][column] =
        leftChars[row] === rightChars[column]
          ? dp[row + 1][column + 1] + 1
          : Math.max(dp[row + 1][column], dp[row][column + 1]);
    }
  }

  const leftSegments: DiffSegment[] = [];
  const rightSegments: DiffSegment[] = [];
  let leftIndex = 0;
  let rightIndex = 0;

  while (leftIndex < rowCount && rightIndex < columnCount) {
    if (leftChars[leftIndex] === rightChars[rightIndex]) {
      pushDiffSegment(leftSegments, leftChars[leftIndex], false);
      pushDiffSegment(rightSegments, rightChars[rightIndex], false);
      leftIndex += 1;
      rightIndex += 1;
    } else if (dp[leftIndex + 1][rightIndex] >= dp[leftIndex][rightIndex + 1]) {
      pushDiffSegment(leftSegments, leftChars[leftIndex], true);
      leftIndex += 1;
    } else {
      pushDiffSegment(rightSegments, rightChars[rightIndex], true);
      rightIndex += 1;
    }
  }

  while (leftIndex < rowCount) {
    pushDiffSegment(leftSegments, leftChars[leftIndex], true);
    leftIndex += 1;
  }

  while (rightIndex < columnCount) {
    pushDiffSegment(rightSegments, rightChars[rightIndex], true);
    rightIndex += 1;
  }

  return {
    left: leftSegments.length ? leftSegments : [{ content: "", changed: false }],
    right: rightSegments.length ? rightSegments : [{ content: "", changed: false }],
  };
}

function pushDiffSegment(segments: DiffSegment[], content: string, changed: boolean) {
  const lastSegment = segments[segments.length - 1];

  if (lastSegment && lastSegment.changed === changed) {
    lastSegment.content += content;
    return;
  }

  segments.push({ content, changed });
}

function getDiffLineClass(line: DiffLine) {
  return {
    "diff-line--added": line.type === "added",
    "diff-line--removed": line.type === "removed",
    "diff-line--changed": line.type === "changed",
    "diff-line--empty": line.type === "empty",
  };
}
</script>

<template>
  <div class="flex flex-col h-full overflow-hidden bg-white dark:bg-dark">
    <NTabs
      v-model:value="activeTab"
      type="line"
      animated
      class="flex-1 flex flex-col overflow-hidden px-4"
      content-style="flex: 1; display: flex; flex-direction: column; overflow: hidden; padding-top: 12px; padding-bottom: 4px;"
    >
      <NTabPane name="editor" tab="编辑器">
        <div class="flex flex-col h-full gap-4">
          <div class="flex items-center justify-between shrink-0">
            <div class="flex items-center gap-2">
              <NTag v-if="activeVersion" type="warning" size="small" round strong>
                当前生效: v{{ activeVersion.version }}
              </NTag>
              <NTag v-else type="info" size="small" round strong>系统默认</NTag>
            </div>
            <NButton
              quaternary
              size="tiny"
              type="primary"
              @click="showDefault = !showDefault"
            >
              {{ showDefault ? "隐藏初始版本" : "查看初始版本" }}
            </NButton>
          </div>

          <div
            v-if="showDefault"
            class="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-100 dark:border-gray-700 shrink-0"
          >
            <div
              class="text-[11px] font-bold mb-1 text-gray-500 uppercase tracking-wider"
            >
              System Default (Read-only)
            </div>
            <div
              class="text-xs text-gray-400 whitespace-pre-wrap max-h-32 overflow-y-auto italic font-mono leading-relaxed"
            >
              {{ promptData.default_prompt }}
            </div>
          </div>

          <div class="flex-1 flex flex-col relative min-h-[220px] overflow-hidden">
            <NInput
              v-model:value="editingPrompt"
              type="textarea"
              :autosize="false"
              placeholder="在这里编写您的个性化 AI 提示词..."
              class="prompt-textarea font-mono text-[13px] flex-1"
              :input-style="{ height: '100%' }"
            />
          </div>

          <div class="shrink-0 space-y-3">
            <div
              class="p-3 bg-primary-50/10 dark:bg-primary-900/5 border border-dashed border-primary-200 dark:border-primary-800 rounded-lg"
            >
              <div class="text-xs text-gray-500 mb-2 font-bold flex items-center gap-1">
                <div class="i-mdi:comment-edit-outline" />
                版本说明
              </div>
              <NInput
                v-model:value="remark"
                placeholder="简单描述本次修改的内容..."
                size="small"
              />
            </div>

            <div class="flex justify-end gap-3 pb-2">
              <NPopconfirm
                @positive-click="editingPrompt = promptData.default_prompt"
              >
                <template #trigger>
                  <NButton
                    ghost
                    :disabled="editingPrompt === promptData.default_prompt"
                  >
                    恢复默认
                  </NButton>
                </template>
                确定要将当前编辑的内容恢复为系统默认提示词吗？
              </NPopconfirm>
              <NButton
                ghost
                :disabled="!hasChanges"
                @click="
                  editingPrompt = promptData.effective_prompt || promptData.default_prompt
                "
              >
                撤销修改
              </NButton>
              <NButton
                type="primary"
                :loading="saving"
                :disabled="!hasChanges"
                class="px-6"
                @click="handleSave"
              >
                发布此版本
              </NButton>
            </div>
          </div>
        </div>
      </NTabPane>

      <NTabPane name="history">
        <template #tab>
          <div class="flex items-center gap-1">
            <span>版本历史</span>
            <NBadge :value="promptData.versions.length" :max="99" type="info" />
          </div>
        </template>

        <div class="flex flex-col h-full overflow-hidden">
          <div
            v-if="promptData.versions.length === 0"
            class="flex-1 flex items-center justify-center"
          >
            <NEmpty description="还没有保存过任何版本" />
          </div>

          <NScrollbar v-else class="flex-1 pr-2">
            <div class="space-y-3">
              <div
                v-for="v in promptData.versions"
                :key="v.id"
                class="group p-4 rounded-xl border transition-all duration-300 hover:shadow-md relative overflow-hidden"
                :class="
                  v.is_active
                    ? 'border-primary-500 bg-primary-50/5 ring-1 ring-primary-500/20'
                    : 'border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-800/50 hover:border-primary-300'
                "
              >
                <!-- 状态标识 -->
                <div v-if="v.is_active" class="absolute top-0 right-0">
                  <div
                    class="bg-primary text-white text-[10px] px-3 py-0.5 rounded-bl-lg font-bold"
                  >
                    ACTIVE
                  </div>
                </div>

                <div class="flex items-center justify-between mb-3">
                  <div class="flex items-center gap-3">
                    <div
                      class="w-10 h-10 rounded-full bg-gray-100 dark:bg-gray-700 flex items-center justify-center font-bold text-gray-500 group-hover:bg-primary-500 group-hover:text-white transition-colors"
                    >
                      v{{ v.version }}
                    </div>
                    <div class="flex flex-col">
                      <span class="text-xs text-gray-400">{{
                        format(new Date(v.created_at), "yyyy-MM-dd HH:mm")
                      }}</span>
                    </div>
                  </div>

                  <div class="flex items-center gap-2">
                    <NPopconfirm
                      placement="left"
                      scroll-strategy="reposition"
                      @positive-click="handleDelete(v.id)"
                    >
                      <template #trigger>
                        <div class="inline-block">
                          <ButtonIcon
                            icon="mdi:trash-can-outline"
                            class="text-gray-400 hover:text-error transition-colors"
                            :disabled="v.is_active && promptData.versions.length === 1"
                            tooltip-content="删除此版本"
                          />
                        </div>
                      </template>
                      确定要永久删除这个版本吗？
                    </NPopconfirm>
                  </div>
                </div>

                <div
                  class="text-[13px] text-gray-600 dark:text-gray-400 mb-4 bg-gray-50/50 dark:bg-gray-900/30 p-2 rounded italic line-clamp-2 border-l-2 border-gray-200"
                >
                  {{ v.remark || "无备注信息" }}
                </div>

                <div class="flex gap-2 justify-end">
                  <NButton size="small" ghost @click="viewHistory(v)"> 载入编辑 </NButton>
                  <NButton
                    v-if="!v.is_active"
                    size="small"
                    type="primary"
                    secondary
                    @click="handleSwitch(v.id)"
                  >
                    启用该版
                  </NButton>
                  <NButton size="small" quaternary type="info" @click="openCompare(v)">
                    对比差异
                  </NButton>
                </div>
              </div>
            </div>
          </NScrollbar>

          <div class="mt-auto pt-6 pb-2 border-t border-gray-100 dark:border-gray-800">
            <NPopconfirm @positive-click="handleReset">
              <template #trigger>
                <NButton
                  block
                  quaternary
                  type="error"
                  size="small"
                  class="opacity-60 hover:opacity-100"
                >
                  <template #icon><div class="i-mdi:refresh" /></template>
                  重置为系统出厂设置
                </NButton>
              </template>
              这将删除您的所有自定义提示词版本并恢复到系统初始状态。
            </NPopconfirm>
          </div>
        </div>
      </NTabPane>
    </NTabs>

    <!-- 版本对比 Modal -->
    <NModal
      v-model:show="showCompareModal"
      preset="card"
      :title="'版本对比: v' + (compareTarget?.version || '') + ' vs 当前生效'"
      style="width: 95%; max-width: 1200px"
      header-style="padding: 16px 24px; border-bottom: 1px solid #f0f0f0;"
    >
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6 h-[60vh] min-h-[400px]">
        <div
          class="flex flex-col h-full overflow-hidden rounded-xl border border-gray-200 dark:border-gray-700 bg-gray-50/30"
        >
          <div
            class="p-3 bg-white dark:bg-gray-800 border-b flex justify-between items-center shrink-0"
          >
            <div class="flex items-center gap-2">
              <div class="w-2 h-2 rounded-full bg-gray-400" />
              <span class="text-sm font-bold text-gray-500">备份版本 (v{{ compareTarget?.version }})</span>
            </div>
            <NTag size="tiny" quaternary>HISTORY</NTag>
          </div>
          <div
            ref="leftScrollRef"
            class="diff-pane flex-1 p-3 overflow-auto text-[13px] font-mono leading-relaxed"
            @scroll="syncScroll($event, 'left')"
          >
            <div
              v-for="(row, index) in compareDiffRows"
              :key="`left-${index}`"
              class="diff-line"
              :class="getDiffLineClass(row.left)"
            >
              <span class="diff-line-number">{{ row.left.lineNumber || "" }}</span>
              <span class="diff-line-content">
                <span
                  v-for="(segment, segmentIndex) in row.left.segments"
                  :key="`left-${index}-${segmentIndex}`"
                  :class="{ 'diff-segment--changed': segment.changed }"
                >{{ segment.content || " " }}</span>
              </span>
            </div>
          </div>
        </div>
        <div
          class="flex flex-col h-full overflow-hidden rounded-xl border border-primary-200 dark:border-primary-800 bg-primary-50/5"
        >
          <div
            class="p-3 bg-white dark:bg-gray-800 border-b flex justify-between items-center shrink-0"
          >
            <div class="flex items-center gap-2">
              <div class="w-2 h-2 rounded-full bg-primary" />
              <span class="text-sm font-bold text-primary">当前生效版本</span>
            </div>
            <NTag size="tiny" type="primary" quaternary>ACTIVE</NTag>
          </div>
          <div
            ref="rightScrollRef"
            class="diff-pane flex-1 p-3 overflow-auto text-[13px] font-mono leading-relaxed"
            @scroll="syncScroll($event, 'right')"
          >
            <div
              v-for="(row, index) in compareDiffRows"
              :key="`right-${index}`"
              class="diff-line"
              :class="getDiffLineClass(row.right)"
            >
              <span class="diff-line-number">{{ row.right.lineNumber || "" }}</span>
              <span class="diff-line-content">
                <span
                  v-for="(segment, segmentIndex) in row.right.segments"
                  :key="`right-${index}-${segmentIndex}`"
                  :class="{ 'diff-segment--changed': segment.changed }"
                >{{ segment.content || " " }}</span>
              </span>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-between items-center w-full px-2">
          <NPopconfirm
            placement="top"
            @positive-click="
              handleDelete(compareTarget.id);
              showCompareModal = false;
            "
          >
            <template #trigger>
              <div class="inline-block">
                <NButton
                  v-if="compareTarget && !compareTarget.is_active"
                  quaternary
                  type="error"
                  size="small"
                >
                  永久删除该备份
                </NButton>
              </div>
            </template>
            确定要删除这个历史备份吗？此操作不可撤销。
          </NPopconfirm>
          <div class="flex gap-3">
            <NButton ghost @click="showCompareModal = false">关闭</NButton>
            <NButton
              v-if="compareTarget && !compareTarget.is_active"
              type="primary"
              class="px-6"
              @click="
                handleSwitch(compareTarget.id);
                showCompareModal = false;
              "
            >
              回退到此版本
            </NButton>
          </div>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
:deep(.n-tabs) {
  height: 100%;
}

:deep(.n-tabs-nav) {
  padding: 0 4px;
  flex-shrink: 0;
}

:deep(.n-tabs-content) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

:deep(.n-tab-pane) {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
}

:deep(.prompt-textarea) {
  display: flex;
  flex: 1;
  height: 100%;
  min-height: 540px;
  max-height: 100%;
  overflow: hidden;
}

:deep(.prompt-textarea .n-input-wrapper),
:deep(.prompt-textarea .n-input__textarea),
:deep(.prompt-textarea .n-input__textarea-el) {
  height: 100%;
  min-height: 0;
}

:deep(.prompt-textarea .n-input__textarea-el) {
  resize: none;
}

.diff-pane {
  background: rgba(249, 250, 251, 0.65);
}

.diff-line {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  min-height: 24px;
  border-left: 3px solid transparent;
  border-radius: 4px;
}

.diff-line-number {
  padding-right: 10px;
  color: #9ca3af;
  font-size: 11px;
  line-height: 24px;
  text-align: right;
  user-select: none;
}

.diff-line-content {
  min-width: 0;
  padding: 1px 8px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.diff-line--added {
  border-left-color: #22c55e;
  background: rgba(34, 197, 94, 0.12);
}

.diff-line--removed {
  border-left-color: #ef4444;
  background: rgba(239, 68, 68, 0.12);
}

.diff-line--changed {
  border-left-color: #f59e0b;
  background: rgba(245, 158, 11, 0.14);
}

.diff-line--empty {
  background: rgba(156, 163, 175, 0.08);
}

.diff-segment--changed {
  border-radius: 3px;
  background: rgba(245, 158, 11, 0.35);
}

:global(.dark) .diff-pane {
  background: rgba(17, 24, 39, 0.28);
}

:global(.dark) .diff-line-number {
  color: #6b7280;
}

:global(.dark) .diff-line--added {
  background: rgba(34, 197, 94, 0.18);
}

:global(.dark) .diff-line--removed {
  background: rgba(239, 68, 68, 0.18);
}

:global(.dark) .diff-line--changed {
  background: rgba(245, 158, 11, 0.2);
}

:global(.dark) .diff-segment--changed {
  background: rgba(245, 158, 11, 0.45);
}

/* 隐藏滚动条但保留功能 (可选) */
.overflow-auto::-webkit-scrollbar {
  width: 6px;
}
.overflow-auto::-webkit-scrollbar-thumb {
  background-color: rgba(0, 0, 0, 0.1);
  border-radius: 3px;
}
</style>
