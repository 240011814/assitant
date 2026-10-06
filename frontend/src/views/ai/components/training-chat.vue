<script setup lang="ts">
import { nextTick, ref, shallowRef, computed, onBeforeUnmount, onMounted, onActivated, watch } from "vue";
import { useFullscreen } from "@vueuse/core";
import { useMessage, NDrawer, NDrawerContent, NModal, NInput } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import {
  fetchHistoryDetail,
  fetchUpdateFavorite,
  fetchUpdateHistoryTitle,
  fetchGenerateShareToken,
} from "@/service/api";
import { fetchGetAIModels, fetchGetUserPrompt, fetchChatStream, fetchToolApproval } from "@/service/api";
import { fetchOrchestrationChatRun, fetchResolveOrchestrationApproval } from "@/service/api";
import { useAuth } from "@/hooks/business/auth";
import { renderMarkdown as renderMarkdownRaw } from "@/utils/markdown";
import { useRoute } from "vue-router";
import PromptEditor from "./prompt-editor.vue";
import VocabModal from "./vocab-modal.vue";
import NoteModal from "./note-modal.vue";
import CourseItemModal from "./course-item-modal.vue";

interface VocabSuggestion {
  word?: string;
  phonetic?: string;
  definition?: string;
  example?: string;
  confusingWords?: string;
}

interface ExpressionSuggestion {
  english: string;
  chinese: string;
}

interface TokenUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
  renderedContent?: string;
  thinkingContent?: string;
  renderedThinking?: string;
  usage?: TokenUsage;
  suggestions?: VocabSuggestion[];
  expressions?: ExpressionSuggestion[];
  isError?: boolean;
  timestamp?: number;
}

const props = withDefaults(
  defineProps<{
    systemPrompt: string;
    initialMessage: string;
    agentId: number;
    trainingType?: string;
    customTrainingId?: number | null;
    inputPlaceholder?: string;
    assistantColor?: string;
    enableVocabulary?: boolean;
    speechLang?: string;
    speechRate?: number;
    /** 编排对话模式: 传编排 ID 时, 复用本聊天界面但走编排引擎运行 */
    orchestrationId?: number | null;
    /** 编排对话模式下的标题 */
    title?: string;
  }>(),
  {
    trainingType: "",
    customTrainingId: null,
    inputPlaceholder: "输入消息...",
    assistantColor: "#2080f0",
    enableVocabulary: false,
    speechLang: "en-US",
    speechRate: 0.9,
    orchestrationId: null,
    title: "",
  }
);

const { hasAuth } = useAuth();
// 编排对话模式: 传入编排 ID 时复用本聊天界面, 但消息走编排引擎
const isOrchestration = computed(() => props.orchestrationId != null && props.orchestrationId > 0);
const appStore = useAppStore();
const containerRef = ref<HTMLElement>();
const { isFullscreen, toggle: toggleFullscreen } = useFullscreen(containerRef);

const renderMarkdown = (content: string) => {
  return renderMarkdownRaw(content).trim();
};

const formatTime = (timestamp?: number) => {
  if (!timestamp) return "";
  const date = new Date(timestamp);
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};

function formatDisplayContent(content: string) {
  if (!props.enableVocabulary) return content;

  // 移除完整的 <vocabs>...</vocabs> 标签
  let cleaned = content.replace(/<vocabs>[\s\S]*?<\/vocabs>/g, "");
  // 移除不完整的 <vocabs> 标签（流式响应时可能出现）
  cleaned = cleaned.replace(/<vocabs>[\s\S]*$/, "");
  
  // 移除完整的 <expressions>...</expressions> 标签
  cleaned = cleaned.replace(/<expressions>[\s\S]*?<\/expressions>/g, "");
  // 移除不完整的 <expressions> 标签（流式响应时可能出现）
  cleaned = cleaned.replace(/<expressions>[\s\S]*$/, "");
  
  return cleaned.trim();
}

const renderMessageContent = (content: string) => {
  return renderMarkdown(formatDisplayContent(content));
};

const parseVocabsFromContent = (content: string): VocabSuggestion[] | undefined => {
  if (!props.enableVocabulary) return undefined;

  const match = content.match(/<vocabs>([\s\S]*?)<\/vocabs>/);
  if (!match?.[1]) return undefined;

  try {
    const vocabs = JSON.parse(match[1]);
    if (Array.isArray(vocabs) && vocabs.length > 0) {
      return vocabs;
    }
  } catch (error) {
    console.error("Failed to parse vocabs:", error);
  }
  return undefined;
};

const parseExpressionsFromContent = (content: string): ExpressionSuggestion[] | undefined => {
  if (!props.enableVocabulary) return undefined;

  const match = content.match(/<expressions>([\s\S]*?)<\/expressions>/);
  if (!match?.[1]) return undefined;

  try {
    const expressions = JSON.parse(match[1]);
    if (Array.isArray(expressions) && expressions.length > 0) {
      return expressions.filter(e => e.english);
    }
  } catch (error) {
    console.error("Failed to parse expressions:", error);
  }
  return undefined;
};

const createChatMessage = (role: ChatMessage["role"], content: string, timestamp?: number): ChatMessage => {
  return {
    role,
    content,
    renderedContent: renderMessageContent(content),
    suggestions: parseVocabsFromContent(content),
    expressions: parseExpressionsFromContent(content),
    timestamp: timestamp || Date.now(),
  };
};

const systemMessage = ref<ChatMessage>({
  role: "system",
  content: props.systemPrompt,
});

const messages = ref<ChatMessage[]>([
  createChatMessage("assistant", props.initialMessage),
]);

const showPromptEditor = ref(false);

async function refreshPrompt() {
  const { data } = await fetchGetUserPrompt(props.agentId);
  if (data) {
    systemMessage.value.content = data.effective_prompt || props.systemPrompt;
  }
}
const historyId = ref<number>(0);
const isFavorite = ref(false);
const shareToken = ref<string | null>(null);
const historyTitle = ref("");
const showTitleModal = ref(false);
const editTitle = ref("");
const inputMessage = ref("");
const isGenerating = ref(false);
// 当前流式请求的中断控制器(用于"停止生成"与组件卸载时中止)
const abortController = shallowRef<AbortController | null>(null);
const scrollbarRef = ref<any>(null);
const message = useMessage();
let scrollFrame = 0;

const modelOptions = ref<{ label: string; value: string }[]>([]);
const selectedModel = ref("");
// 本次运行发起时使用的模型: 工具审批恢复时随请求带回, 保证用同一模型续跑
let lastRunModel = "";

async function loadModels() {
  const { data } = await fetchGetAIModels();
  if (data && data.length > 0) {
    modelOptions.value = data.map((m) => ({
      label: m.display_name,
      value: m.model_code,
    }));
    // 默认选择第一个模型（或标记为  default的模型）
    const defaultModel = data.find((m) => m.is_default) || data[0];
    selectedModel.value = defaultModel.model_code;
  }
}

// 生词本/笔记/课程包弹窗抽为子组件 (与流式渲染无关的表单块), 经 ref 调用 open()
const vocabModalRef = ref<InstanceType<typeof VocabModal>>();
const noteModalRef = ref<InstanceType<typeof NoteModal>>();
const courseModalRef = ref<InstanceType<typeof CourseItemModal>>();

const showToolApprovalModal = ref(false);
const toolApprovalInfo = ref<{
  toolName: string;
  arguments: string;
  checkpointId: string;
  interruptId: string;
} | null>(null);

// 编排对话的工具审批: 与普通对话的 ADK 恢复机制不同, 编排运行流保持打开,
// 决定经 /ai-orchestrations/approvals/resolve 提交后原流继续
const orchApprovalInfo = ref<{
  runId: string;
  callId: string;
  tool: string;
  arguments: string;
} | null>(null);
const orchApproving = ref(false);

const handleOrchApproval = async (approved: boolean) => {
  if (!orchApprovalInfo.value) return;
  const { runId, callId } = orchApprovalInfo.value;
  orchApproving.value = true;
  try {
    const { error } = await fetchResolveOrchestrationApproval({
      runId,
      callId,
      approved,
      reason: approved ? "" : "用户拒绝执行该工具"
    });
    if (error) {
      message.error(error.message || "提交审批决定失败");
      return;
    }
    orchApprovalInfo.value = null;
  } finally {
    orchApproving.value = false;
  }
};

const route = useRoute();
const routeTitleMap: Record<string, string> = {
  ai_exercise: "练习",
};

const openNoteModal = (content: string) => {
  const routeName = (route.name as string) || "";
  const defaultCategory = (routeTitleMap[routeName] || "未分类").replace("训练", "");

  // 提取前20个字符作为默认标题
  let defaultTitle = content.trim().slice(0, 20);
  if (content.trim().length > 20) defaultTitle += "...";

  noteModalRef.value?.open({
    title: defaultTitle,
    category: defaultCategory,
    content: formatDisplayContent(content),
  });
};

const openVocabModal = () => {
  vocabModalRef.value?.open();
};

const handleSelectText = () => {
  if (!props.enableVocabulary) return;

  const selection = window.getSelection()?.toString().trim();
  if (selection && selection.length > 0 && selection.length < 50) {
    vocabModalRef.value?.setWord(selection);
  }
};

const scrollToBottom = async (behavior: ScrollBehavior = "auto") => {
  await nextTick();
  scrollbarRef.value?.scrollTo({ position: "bottom", behavior });
};

const scrollToTop = async (behavior: ScrollBehavior = "auto") => {
  await nextTick();
  scrollbarRef.value?.scrollTo({ position: "top", behavior });
};

const scheduleScrollToBottom = () => {
  if (scrollFrame) return;

  scrollFrame = window.requestAnimationFrame(() => {
    scrollFrame = 0;
    scrollToBottom();
  });
};

const appendAssistantContent = (content: string) => {
  const lastIdx = messages.value.length - 1;
  const nextContent = messages.value[lastIdx].content + content;
  messages.value[lastIdx] = {
    ...messages.value[lastIdx],
    content: nextContent,
    renderedContent: renderMessageContent(nextContent),
  };
};

// 用最终文本替换当前助手气泡 (编排运行结束后只保留图级最终输出)
const setAssistantContent = (content: string) => {
  const lastIdx = messages.value.length - 1;
  messages.value[lastIdx] = {
    ...messages.value[lastIdx],
    content,
    renderedContent: renderMessageContent(content),
  };
};

const appendUsage = (usage: TokenUsage) => {
  const lastIdx = messages.value.length - 1;
  messages.value[lastIdx] = {
    ...messages.value[lastIdx],
    usage,
  };
};

// 思考过程展开状态管理
const expandedThinking = ref<Set<number>>(new Set());
const toggleThinking = (index: number) => {
  if (expandedThinking.value.has(index)) {
    expandedThinking.value.delete(index);
  } else {
    expandedThinking.value.add(index);
  }
};

const hasAnyThinkingContent = computed(() => messages.value.some((msg) => msg.thinkingContent));

const allThinkingExpanded = computed(() => {
  const thinkingIndices = messages.value
    .map((msg, idx) => (msg.thinkingContent ? idx : -1))
    .filter((idx) => idx !== -1);
  return thinkingIndices.length > 0 && thinkingIndices.every((idx) => expandedThinking.value.has(idx));
});

const toggleAllThinking = () => {
  if (allThinkingExpanded.value) {
    expandedThinking.value.clear();
  } else {
    messages.value.forEach((msg, idx) => {
      if (msg.thinkingContent) {
        expandedThinking.value.add(idx);
      }
    });
  }
};

const appendThinkingContent = (content: string) => {
  const lastIdx = messages.value.length - 1;
  const nextThinking = (messages.value[lastIdx].thinkingContent || "") + content;
  messages.value[lastIdx] = {
    ...messages.value[lastIdx],
    thinkingContent: nextThinking,
    renderedThinking: renderMarkdown(nextThinking),
  };
  // 默认展开思考过程
  expandedThinking.value.add(lastIdx);
};

const setAssistantError = (content: string) => {
  const lastIdx = messages.value.length - 1;
  messages.value[lastIdx] = {
    ...messages.value[lastIdx],
    content,
    renderedContent: renderMessageContent(content),
    isError: true,
  };
};

// 停止生成: 保留已收到的部分回复并追加标注(不标记为错误)
const markAssistantStopped = () => {
  const lastIdx = messages.value.length - 1;
  const lastMsg = messages.value[lastIdx];
  const content = lastMsg.content ? `${lastMsg.content}\n\n（已停止生成）` : '已停止生成';
  messages.value[lastIdx] = {
    ...lastMsg,
    content,
    renderedContent: renderMessageContent(content),
    isError: false,
  };
};

const handleStopGeneration = () => {
  abortController.value?.abort();
};

const setAssistantThinking = (thinking: string) => {
  const lastIdx = messages.value.length - 1;
  const lastMsg = messages.value[lastIdx];
  const existingThinking = lastMsg.thinkingContent || "";
  messages.value[lastIdx] = {
    ...lastMsg,
    thinkingContent: existingThinking + thinking,
    renderedThinking: renderMarkdown(existingThinking + thinking),
  };
  expandedThinking.value.add(lastIdx);
};

const copyToClipboard = async (content: string) => {
  try {
    await navigator.clipboard.writeText(content);
    message.success("已复制");
  } catch {
    message.error("复制失败");
  }
};

const parseVocabSuggestions = () => {
  const lastIdx = messages.value.length - 1;
  const lastMsg = messages.value[lastIdx];
  if (lastMsg.role !== "assistant") return;

  messages.value[lastIdx] = {
    ...lastMsg,
    renderedContent: renderMessageContent(lastMsg.content),
    suggestions: parseVocabsFromContent(lastMsg.content),
    expressions: parseExpressionsFromContent(lastMsg.content),
  };
};

// 编排对话: 历史轮次 (不含本轮 input), 跳过开场欢迎语, 只保留有效的 user/assistant 文本。
// 会话打开即全新, 继续旧对话走历史列表「继续训练」(路由带 history_id), 不做自动恢复。
const buildOrchestrationHistory = () => {
  const turns: { role: string; content: string }[] = [];
  messages.value.slice(0, -2).forEach((msg, idx) => {
    if (msg.role !== "user" && msg.role !== "assistant") return;
    if (!msg.content.trim() || msg.isError) return;
    // 跳过首条开场白 (assistant 欢迎语), 避免污染多轮上下文
    if (idx === 0 && msg.role === "assistant") return;
    turns.push({ role: msg.role, content: msg.content });
  });
  return turns;
};

// 编排对话: 调用编排运行时并以 SSE 流式渲染 (事件 delta/reasoning/summary/error)
const sendOrchestrationMessage = async (userText: string, controller: AbortController) => {
  const response = await fetchOrchestrationChatRun({
    id: props.orchestrationId as number,
    input: userText,
    history: buildOrchestrationHistory(),
    historyId: historyId.value,
    signal: controller.signal
  });

  if (!response.ok) {
    let errorMessage = `请求失败 (HTTP ${response.status})`;
    try {
      const errorData = await response.json();
      if (errorData?.error) errorMessage = errorData.error;
      else if (errorData?.message) errorMessage = errorData.message;
    } catch {}
    throw new Error(errorMessage);
  }

  const reader = response.body?.getReader();
  const decoder = new TextDecoder("utf-8");
  if (!reader) throw new Error("无法获取响应流");

  const handleEvent = (eventType: string, dataStr: string) => {
    let payload: any = null;
    try {
      payload = dataStr ? JSON.parse(dataStr) : null;
    } catch {
      payload = null;
    }

    switch (eventType) {
      case "history_id":
        // 编排对话首轮落库后返回 history_id, 记录以便后续轮次保存到同一会话
        if (payload?.history_id) {
          historyId.value = payload.history_id;
        }
        break;
      case "error": {
        const msg =
          payload?.message ||
          (Array.isArray(payload?.errors) ? payload.errors.join("; ") : "") ||
          "编排执行失败";
        setAssistantError(`AI 服务错误: ${msg}`);
        break;
      }
      case "delta":
        if (payload?.content) {
          appendAssistantContent(payload.content);
          scheduleScrollToBottom();
        }
        break;
      case "reasoning":
        if (payload?.content) {
          appendThinkingContent(payload.content);
          scheduleScrollToBottom();
        }
        break;
      case "approval_request":
        // 需人工确认的工具: 弹窗等待用户决定 (运行流阻塞在服务端)
        orchApprovalInfo.value = {
          runId: String(payload?.run_id || ""),
          callId: String(payload?.call_id || ""),
          tool: String(payload?.tool || ""),
          arguments: String(payload?.arguments || ""),
        };
        break;
      case "approval_result":
        if (orchApprovalInfo.value?.callId === String(payload?.call_id || "")) {
          orchApprovalInfo.value = null;
        }
        break;
      case "summary":
        // 编排运行会把"工具调用轮次的前言文本 / 子Agent 输出 / 最终答案"都混进同一个气泡,
        // 这里用图级最终输出 (summary.output) 覆盖, 保证只保留最终答案。
        // 停止生成时不会有 summary, 期间流式内容保留。
        if (typeof payload?.output === "string" && payload.output.trim()) {
          setAssistantContent(payload.output);
        }
        if (payload?.tokens) appendUsage(payload.tokens);
        scheduleScrollToBottom();
        break;
      default:
        break;
    }
  };

  let buffer = "";
  let eventType = "message";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() || "";

    for (let line of lines) {
      line = line.trim();
      if (!line) continue;

      if (line.startsWith("event:")) {
        eventType = line.replace(/^event:\s*/, "").trim();
        continue;
      }

      if (line.startsWith("data:")) {
        const dataStr = line.replace(/^data:\s*/, "").trim();
        handleEvent(eventType, dataStr);
      }
    }
  }
};

const sendMessage = async () => {
  if (!inputMessage.value.trim() || isGenerating.value) return;

  const userText = inputMessage.value;
  inputMessage.value = "";

  messages.value.push(createChatMessage("user", userText));
  messages.value.push(createChatMessage("assistant", ""));
  // 上一轮若残留审批弹窗 (已中止的运行), 新一轮开始时清掉
  orchApprovalInfo.value = null;

  scrollToBottom();
  isGenerating.value = true;

  const controller = new AbortController();
  abortController.value = controller;

  try {
    if (isOrchestration.value) {
      await sendOrchestrationMessage(userText, controller);
      return;
    }

    const routeName = props.trainingType || (route.name as string) || "ai_agent";
    const history = messages.value
      .slice(0, -1)
      .filter((item) => item.content.trim() && !item.isError);
    const apiMessages = history.map(({ role, content }) => ({ role, content }));

    lastRunModel = selectedModel.value;
    const response = await fetchChatStream({
      history_id: historyId.value,
      training_type: routeName,
      custom_training_id: props.customTrainingId || undefined,
      agent_id: props.agentId,
      model: selectedModel.value,
      messages: apiMessages,
      signal: controller.signal,
    });

    if (!response.ok) {
      let errorMessage = `请求失败 (HTTP ${response.status})`;
      try {
        const errorData = await response.json();
        if (errorData.error) errorMessage = errorData.error;
      } catch {}
      throw new Error(errorMessage);
    }

    const reader = response.body?.getReader();
    const decoder = new TextDecoder("utf-8");
    if (!reader) throw new Error("无法获取响应流");

    let buffer = "";
    let eventType = "message"; // 1. 移到循环外，持久化状态

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() || "";

      for (let line of lines) {
        line = line.trim();
        if (!line) continue;

        if (line.startsWith("event:")) {
          eventType = line.replace(/^event:\s*/, "").trim();
          continue;
        }

        if (line.startsWith("data:")) {
          const dataStr = line.replace(/^data:\s*/, "").trim();

          try {
            const dataObj = JSON.parse(dataStr);

            if (eventType == "history_id") {
              historyId.value = dataObj.history_id;
              if (dataObj.title) {
                historyTitle.value = dataObj.title;
              }
            }

            if (eventType === "tool_approval") {
              toolApprovalInfo.value = {
                toolName: dataObj.tool_name,
                arguments: dataObj.arguments,
                checkpointId: dataObj.checkpoint_id,
                interruptId: dataObj.interrupt_id,
              };
              showToolApprovalModal.value = true;
              return;
            }

            if (dataObj.error) {
              setAssistantError(`AI 服务错误: ${dataObj.error}`);
              return;
            }

            if (dataObj.reasoning_content) {
              appendThinkingContent(dataObj.reasoning_content);
              scheduleScrollToBottom();
            }

            if (dataObj.thinking) {
              setAssistantThinking(dataObj.thinking);
              scheduleScrollToBottom();
            }

            if (dataObj.content) {
              appendAssistantContent(dataObj.content);
              scheduleScrollToBottom();
            }

            if (dataObj.usage) {
              appendUsage(dataObj.usage);
            }
          } catch (e) {
            console.warn("Parse error:", e);
          }
        }
      }
    }
  } catch (err: any) {
    if (controller.signal.aborted) {
      // 用户主动停止: 保留已收到的部分回复并标注, 不作为错误处理
      markAssistantStopped();
    } else {
      setAssistantError(`连接 AI 服务失败: ${err?.message || "未知错误"}。`);
      isGenerating.value = false;
    }
  } finally {
    isGenerating.value = false;
    if (abortController.value === controller) {
      abortController.value = null;
    }
    await scrollToBottom();
    parseVocabSuggestions();
  }
};

const handleEnter = (event: KeyboardEvent) => {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    sendMessage();
  }
};

// 对话中最后一条 user 消息的下标(用于"编辑重发", -1 表示不存在)
const lastUserMessageIndex = computed(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    if (messages.value[i].role === 'user') return i;
  }
  return -1;
});

// 编辑重发: 将该条 user 消息内容回填输入框, 并从该条起截断之后的全部消息; 重新发送走现有 sendMessage
const handleEditMessage = (index: number) => {
  if (isGenerating.value) return;
  const msg = messages.value[index];
  if (!msg || msg.role !== 'user') return;
  inputMessage.value = msg.content;
  messages.value.splice(index);
  scrollToBottom();
};

// 失败重试: 移除错误回复及其后的消息, 以其前一条 user 消息内容重新发送
const handleRetryMessage = (index: number) => {
  if (isGenerating.value) return;
  const msg = messages.value[index];
  if (!msg || msg.role !== 'assistant' || !msg.isError) return;
  let userIdx = -1;
  for (let i = index - 1; i >= 0; i--) {
    if (messages.value[i].role === 'user') {
      userIdx = i;
      break;
    }
  }
  if (userIdx < 0) return;
  const userContent = messages.value[userIdx].content;
  messages.value.splice(userIdx);
  inputMessage.value = userContent;
  sendMessage();
};

const handleApplySuggestion = (vocab: VocabSuggestion) => {
  vocabModalRef.value?.open({
    word: vocab.word || "",
    phonetic: vocab.phonetic || "",
    definition: vocab.definition || "",
    example: vocab.example || "",
    confusingWords: vocab.confusingWords || "",
  });
};

const handleAddExpression = (expr: ExpressionSuggestion) => {
  courseModalRef.value?.open({
    english_sentence: expr.english,
    chinese_translation: expr.chinese || "",
  });
};

const handlePlay = (text: string) => {
  if (!window.speechSynthesis) {
    message.error("您的浏览器不支持语音播放");
    return;
  }

  window.speechSynthesis.cancel();
  const utterance = new SpeechSynthesisUtterance(text);
  utterance.lang = props.speechLang;
  utterance.rate = props.speechRate;
  window.speechSynthesis.speak(utterance);
};

const handleToolApproval = async (approved: boolean, reason?: string) => {
  if (!toolApprovalInfo.value) return;

  const { checkpointId, interruptId } = toolApprovalInfo.value;
  showToolApprovalModal.value = false;
  isGenerating.value = true;

  const routeName = props.trainingType || (route.name as string) || "ai_agent";
  const history = messages.value
    .slice(0, -1)
    .filter((item) => item.content.trim() && !item.isError);
  const apiMessages = history.map(({ role, content }) => ({ role, content }));

  try {
    const response = await fetchToolApproval({
      checkpoint_id: checkpointId,
      interrupt_id: interruptId,
      approved,
      reason,
      history_id: historyId.value,
      training_type: routeName,
      custom_training_id: props.customTrainingId || undefined,
      agent_id: props.agentId,
      model: lastRunModel,
      messages: apiMessages,
    });

    if (!response.ok) {
      let errorMessage = `请求失败 (HTTP ${response.status})`;
      try {
        const errorData = await response.json();
        if (errorData.error) errorMessage = errorData.error;
      } catch {}
      throw new Error(errorMessage);
    }

    const reader = response.body?.getReader();
    const decoder = new TextDecoder("utf-8");
    if (!reader) throw new Error("无法获取响应流");

    let buffer = "";
    let eventType = "message";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() || "";

      for (let line of lines) {
        line = line.trim();
        if (!line) continue;

        if (line.startsWith("event:")) {
          eventType = line.replace(/^event:\s*/, "").trim();
          continue;
        }

        if (line.startsWith("data:")) {
          const dataStr = line.replace(/^data:\s*/, "").trim();

          try {
            const dataObj = JSON.parse(dataStr);

            if (eventType === "tool_approval") {
              toolApprovalInfo.value = {
                toolName: dataObj.tool_name,
                arguments: dataObj.arguments,
                checkpointId: dataObj.checkpoint_id,
                interruptId: dataObj.interrupt_id,
              };
              showToolApprovalModal.value = true;
              return;
            }

            if (dataObj.error) {
              setAssistantError(`AI 服务错误: ${dataObj.error}`);
              return;
            }

            if (dataObj.reasoning_content) {
              appendThinkingContent(dataObj.reasoning_content);
              scheduleScrollToBottom();
            }

            if (dataObj.thinking) {
              setAssistantThinking(dataObj.thinking);
              scheduleScrollToBottom();
            }

            if (dataObj.content) {
              appendAssistantContent(dataObj.content);
              scheduleScrollToBottom();
            }
          } catch (e) {
            console.warn("Parse error:", e);
          }
        }
      }
    }
  } catch (err: any) {
    setAssistantError(`工具审批请求失败: ${err?.message || "未知错误"}`);
  } finally {
    isGenerating.value = false;
    await scrollToBottom();
  }
};

const lastLoadedHistoryId = ref<number>(0);

const loadHistory = async (id: number) => {
  try {
    const { data } = await fetchHistoryDetail(id);
    if (data) {
      historyId.value = data.id;
      lastLoadedHistoryId.value = data.id;
      historyTitle.value = data.title;
      messages.value = (data.messages || [])
        .filter((msg: any) => msg.role !== "system")
        .map((msg: any, idx: number) => {
          const chatMsg = createChatMessage(msg.role, msg.content);
          if (msg.thinking_content) {
            chatMsg.thinkingContent = msg.thinking_content;
            chatMsg.renderedThinking = renderMarkdown(msg.thinking_content);
            // 历史记录中的思考过程默认展开
            expandedThinking.value.add(idx);
          }
          return chatMsg;
        });
      isFavorite.value = data.is_favorite;
      shareToken.value = data.share_token || null;
    }
  } catch (err: any) {
    message.error(`加载历史记录失败: ${err?.message || "未知错误"}`);
  }
};

const handleToggleFavorite = async () => {
  if (!historyId.value) {
    message.warning("请先发送消息后再收藏");
    return;
  }
  try {
    await fetchUpdateFavorite(historyId.value, !isFavorite.value);
    isFavorite.value = !isFavorite.value;
    message.success(isFavorite.value ? "已收藏" : "已取消收藏");
  } catch (err: any) {
    message.error(`操作失败: ${err?.message || "未知错误"}`);
  }
};

const getShareUrl = (token: string) => {
  const isHashMode = import.meta.env.VITE_ROUTER_HISTORY_MODE === "hash";
  const basePath = isHashMode ? "/#/" : "/";
  return `${window.location.origin}${basePath}share/${token}`;
};

const handleShare = async () => {
  if (!historyId.value) {
    message.warning("请先发送消息后再分享");
    return;
  }
  try {
    if (shareToken.value) {
      const shareUrl = getShareUrl(shareToken.value);
      await navigator.clipboard.writeText(shareUrl);
      message.success("分享链接已复制到剪贴板");
    } else {
      const { data } = await fetchGenerateShareToken(historyId.value);
      if (data?.share_token) {
        shareToken.value = data.share_token;
        const shareUrl = getShareUrl(data.share_token);
        await navigator.clipboard.writeText(shareUrl);
        message.success("分享链接已复制到剪贴板");
      }
    }
  } catch (err: any) {
    message.error(`操作失败: ${err?.message || "未知错误"}`);
  }
};

const handleOpenEditTitle = () => {
  if (!historyId.value) {
    message.warning("请先发送消息后再编辑标题");
    return;
  }
  editTitle.value = historyTitle.value;
  showTitleModal.value = true;
};

const handleSaveTitle = async () => {
  if (!editTitle.value.trim()) {
    message.warning("标题不能为空");
    return;
  }
  try {
    await fetchUpdateHistoryTitle(historyId.value, editTitle.value.trim());
    historyTitle.value = editTitle.value.trim();
    showTitleModal.value = false;
    message.success("标题已更新");
  } catch (err: any) {
    message.error(`操作失败: ${err?.message || "未知错误"}`);
  }
};

onMounted(() => {
  // 编排对话模式下不加载模型列表/用户提示词 (运行由编排定义决定)
  if (!isOrchestration.value) {
    loadModels();
    refreshPrompt();
  }

  const queryHistoryId = Number(route.query.history_id) || 0;
  if (queryHistoryId) {
    // 仅显式续聊 (历史列表「继续训练」带 history_id) 时回放会话; 打开即新会话
    loadHistory(queryHistoryId);
  }

  if (scrollbarRef.value) {
    scrollbarRef.value.scrollTo({ top: 999999 });
  }
});

onActivated(() => {
  const queryHistoryId = Number(route.query.history_id) || 0;
  if (queryHistoryId && queryHistoryId !== lastLoadedHistoryId.value) {
    messages.value = [];
    loadHistory(queryHistoryId);
  }
});

// 同一标签内切换编排时, 重置为全新会话 (继续旧对话走历史列表「继续训练」)
watch(
  () => props.orchestrationId,
  (newId, oldId) => {
    if (newId == null || newId === oldId) return;
    historyId.value = 0;
    historyTitle.value = "";
    lastLoadedHistoryId.value = 0;
    messages.value = [createChatMessage("assistant", props.initialMessage)];
  }
);

onBeforeUnmount(() => {
  // 组件卸载时中止进行中的流式请求, 释放 AbortController
  abortController.value?.abort();
  abortController.value = null;
  if (scrollFrame) {
    window.cancelAnimationFrame(scrollFrame);
    scrollFrame = 0;
  }
});
</script>

<template>
  <div
    ref="containerRef"
    class="h-full flex-col flex overflow-hidden"
    :class="appStore.isMobile ? 'p-1 gap-1' : 'p-4 gap-4'"
  >
    <NCard
      class="flex-1 overflow-hidden"
      content-class="p-0 flex flex-col overflow-hidden"
      :bordered="false"
      :shadow="appStore.isMobile ? false : 'sm'"
    >
      <!-- Header - Gemini style minimal -->
      <div
        class="flex items-center justify-between border-b border-gray-100 dark:border-gray-800"
        :class="appStore.isMobile ? 'px-3 py-2' : 'px-6 py-3'"
      >
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <span
            class="font-semibold text-gray-700 dark:text-gray-300 truncate"
            :class="appStore.isMobile ? 'text-sm' : 'text-base'"
          >{{ historyTitle || props.title || "AI 训练对话" }}</span>
          <NButton v-if="!isOrchestration" quaternary size="tiny" @click="handleOpenEditTitle">
            <template #icon>
              <SvgIcon
                icon="mdi:pencil-outline"
                class="text-gray-400 hover:text-primary"
              />
            </template>
          </NButton>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <NButton
            v-if="appStore.isMobile"
            quaternary
            size="small"
            @click="toggleFullscreen"
          >
            <template #icon>
              <SvgIcon :icon="isFullscreen ? 'mdi:fullscreen-exit' : 'mdi:fullscreen'" />
            </template>
          </NButton>
          <NButton
            quaternary
            size="small"
            :type="isFavorite ? 'warning' : 'default'"
            @click="handleToggleFavorite"
          >
            <template #icon>
              <SvgIcon :icon="isFavorite ? 'mdi:star' : 'mdi:star-outline'" />
            </template>
          </NButton>
          <NButton quaternary size="small" @click="handleShare">
            <template #icon>
              <SvgIcon icon="mdi:share-variant" />
            </template>
          </NButton>
          <NButton
            v-if="hasAuth('ai:prompt:manage') && !isOrchestration"
            quaternary
            size="small"
            @click="showPromptEditor = true"
          >
            <template #icon>
              <SvgIcon icon="mdi:cog-outline" />
            </template>
          </NButton>
        </div>
      </div>

      <div class="relative flex-1 overflow-hidden">
        <NScrollbar
          ref="scrollbarRef"
          class="flex-1 bg-gray-50/50 dark:bg-dark"
          :class="appStore.isMobile ? 'px-1.5 py-1' : 'p-4'"
        >
          <div class="flex flex-col pb-4" :class="appStore.isMobile ? 'gap-4' : 'gap-6'">
            <div
              v-for="(msg, index) in messages"
              :key="index"
              class="flex items-start"
              :class="[
                msg.role === 'user' ? 'flex-row-reverse' : 'flex-row',
                appStore.isMobile ? 'gap-2' : 'gap-3',
              ]"
            >
              <!-- PC: 水平布局（头像+气泡并排） -->
              <template v-if="!appStore.isMobile">
                <NAvatar
                  :color="msg.role === 'user' ? '#6bb8e8' : assistantColor"
                  round
                  size="large"
                  class="shrink-0 self-start"
                >
                  {{ msg.role === "user" ? "U" : "AI" }}
                </NAvatar>
                <div class="flex flex-col gap-1 max-w-[80%]">
                  <div class="group/btn">
                    <!-- 思考过程 -->
                    <div v-if="msg.role === 'assistant' && msg.thinkingContent" class="thinking-wrapper mb-2">
                      <button
                        class="thinking-toggle"
                        @click="toggleThinking(index)"
                      >
                        <span class="thinking-toggle-icon" :class="{ expanded: expandedThinking.has(index) }">▶</span>
                        <span>💭 思考过程</span>
                      </button>
                      <Transition name="thinking-expand">
                        <div v-if="expandedThinking.has(index)" class="thinking-body">
                          <!-- eslint-disable-next-line vue/no-v-html -->
                          <div class="thinking-content" v-html="msg.renderedThinking"></div>
                        </div>
                      </Transition>
                    </div>
                    <div
                      class="p-4 text-[15px] rounded-2xl whitespace-pre-wrap leading-relaxed shadow-sm"
                      :class="
                        msg.role === 'user'
                          ? 'bg-[#e8f4fd] text-gray-800 rounded-tr-none dark:bg-blue-900/40 dark:text-gray-200'
                          : 'bg-white text-gray-800 rounded-tl-none dark:bg-gray-800 dark:text-gray-200'
                      "
                      @mouseup="msg.role === 'assistant' ? handleSelectText() : undefined"
                    >
                      <!-- eslint-disable-next-line vue/no-v-html -->
                      <div class="msg-content" v-html="msg.renderedContent"></div>
                      <span
                        v-if="
                          isGenerating &&
                            index === messages.length - 1 &&
                            msg.content === ''
                        "
                        class="inline-block mt-1"
                      >
                        <NSpin size="small" />
                      </span>
                      <span
                        v-else-if="
                          isGenerating &&
                            index === messages.length - 1 &&
                            msg.content !== ''
                        "
                        class="inline-flex items-center gap-1.5 ml-1 align-bottom"
                      >
                        <span class="thinking-dot" style="animation-delay: 0s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.15s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.3s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.45s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.6s"></span>
                      </span>
                    </div>

                    <div
                      class="flex items-center gap-2 mt-1 justify-end"
                    >
                      <span v-if="msg.usage" class="text-[11px] text-gray-400 dark:text-gray-500">
                        Token: {{ msg.usage.total_tokens }}
                        <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
                        输入 {{ msg.usage.prompt_tokens }} / 输出 {{ msg.usage.completion_tokens }}
                        <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
                      </span>
                      <span class="text-[11px] text-gray-400 dark:text-gray-500">
                        {{ formatTime(msg.timestamp) }}
                      </span>
                    </div>
                    <div
                      v-if="msg.content"
                      class="flex items-center gap-0.5 mt-1 justify-end opacity-0 group-hover/btn:opacity-100 transition-all duration-200"
                    >
                      <ButtonIcon
                        v-if="msg.role === 'user' && index === lastUserMessageIndex && !isGenerating"
                        icon="mdi:pencil-outline"
                        class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
                        tooltip-content="编辑"
                        @click.stop="handleEditMessage(index)"
                      />
                      <ButtonIcon
                        v-if="msg.role === 'assistant' && msg.isError && !isGenerating"
                        icon="mdi:refresh"
                        class="!h-28px !w-28px text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400"
                        tooltip-content="重试"
                        @click.stop="handleRetryMessage(index)"
                      />
                      <ButtonIcon
                        icon="mdi:content-copy"
                        class="!h-28px !w-28px text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
                        tooltip-content="复制"
                        @click.stop="copyToClipboard(msg.content)"
                      />
                      <ButtonIcon
                        v-if="enableVocabulary && msg.role === 'assistant'"
                        icon="mdi:star-outline"
                        class="!h-28px !w-28px text-gray-400 hover:text-amber-500 dark:text-gray-500 dark:hover:text-amber-400"
                        tooltip-content="添加到生词本"
                        @click.stop="openVocabModal()"
                      />
                      <ButtonIcon
                        v-if="msg.role === 'assistant'"
                        icon="mdi:notebook-edit-outline"
                        class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
                        tooltip-content="添加笔记"
                        @click.stop="openNoteModal(msg.content)"
                      />
                    </div>
                  </div>

                  <div
                    v-if="enableVocabulary && msg.suggestions?.length"
                    class="flex flex-wrap gap-2 mt-2"
                  >
                    <span class="text-xs text-gray-400 self-center">智能建议:</span>
                    <NTag
                      v-for="(vocab, vocabIndex) in msg.suggestions"
                      :key="vocabIndex"
                      size="small"
                      round
                      type="info"
                      check-strategy="child"
                      class="cursor-pointer hover:shadow-sm transition-shadow"
                      @click="handleApplySuggestion(vocab)"
                    >
                      <template #icon>
                        <div class="i-mdi:plus" />
                      </template>
                      {{ vocab.word }}
                    </NTag>
                  </div>

                  <div
                    v-if="hasAuth('ai:course:edit') && msg.expressions?.length"
                    class="flex flex-wrap gap-2 mt-2"
                  >
                    <span class="text-xs text-gray-400 self-center">添加到课程:</span>
                    <NTag
                      v-for="(expr, exprIndex) in msg.expressions"
                      :key="exprIndex"
                      size="small"
                      round
                      type="success"
                      check-strategy="child"
                      class="cursor-pointer hover:shadow-sm transition-shadow"
                      @click="handleAddExpression(expr)"
                    >
                      <template #icon>
                        <div class="i-mdi:plus" />
                      </template>
                      {{ expr.english.slice(0, 30) }}{{ expr.english.length > 30 ? '...' : '' }}
                    </NTag>
                  </div>
                </div>
              </template>

              <!-- 移动端: 垂直布局（头像独占一行，气泡占满宽度） -->
              <template v-else>
                <div class="flex flex-col gap-2 w-full">
                  <div
                    class="flex"
                    :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
                  >
                    <div
                      class="w-8 h-8 rounded-full flex items-center justify-center text-white text-xs font-bold flex-shrink-0"
                      :style="{
                        backgroundColor: msg.role === 'user' ? '#6bb8e8' : assistantColor,
                      }"
                    >
                      {{ msg.role === "user" ? "U" : "AI" }}
                    </div>
                  </div>
                  <div
                    class="flex"
                    :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
                  >
                    <!-- 思考过程 -->
                    <div v-if="msg.role === 'assistant' && msg.thinkingContent" class="thinking-wrapper mb-2">
                      <button
                        class="thinking-toggle"
                        @click="toggleThinking(index)"
                      >
                        <span class="thinking-toggle-icon" :class="{ expanded: expandedThinking.has(index) }">▶</span>
                        <span>💭 思考过程</span>
                      </button>
                      <Transition name="thinking-expand">
                        <div v-if="expandedThinking.has(index)" class="thinking-body">
                          <!-- eslint-disable-next-line vue/no-v-html -->
                          <div class="thinking-content" v-html="msg.renderedThinking"></div>
                        </div>
                      </Transition>
                    </div>
                    <div
                      class="p-3 text-[14px] rounded-2xl whitespace-pre-wrap leading-relaxed shadow-sm w-full"
                      :class="
                        msg.role === 'user'
                          ? 'bg-[#e8f4fd] text-gray-800 rounded-tr-none dark:bg-blue-900/40 dark:text-gray-200'
                          : 'bg-white text-gray-800 rounded-tl-none dark:bg-gray-800 dark:text-gray-200'
                      "
                      @mouseup="msg.role === 'assistant' ? handleSelectText() : undefined"
                    >
                      <!-- eslint-disable-next-line vue/no-v-html -->
                      <div class="msg-content" v-html="msg.renderedContent"></div>
                      <span
                        v-if="
                          isGenerating &&
                            index === messages.length - 1 &&
                            msg.content === ''
                        "
                        class="inline-block mt-1"
                      >
                        <NSpin size="small" />
                      </span>
                      <span
                        v-else-if="
                          isGenerating &&
                            index === messages.length - 1 &&
                            msg.content !== ''
                        "
                        class="inline-flex items-center gap-1.5 ml-1 align-bottom"
                      >
                        <span class="thinking-dot" style="animation-delay: 0s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.15s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.3s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.45s"></span>
                        <span class="thinking-dot" style="animation-delay: 0.6s"></span>
                      </span>
                    </div>
                  </div>
                  <div class="flex items-center gap-2 justify-end">
                    <span v-if="msg.usage" class="text-[11px] text-gray-400 dark:text-gray-500">
                      Token: {{ msg.usage.total_tokens }}
                      <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
                      输入 {{ msg.usage.prompt_tokens }} / 输出 {{ msg.usage.completion_tokens }}
                      <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
                    </span>
                    <span class="text-[11px] text-gray-400 dark:text-gray-500">
                      {{ formatTime(msg.timestamp) }}
                    </span>
                  </div>
                  <div v-if="msg.content" class="flex items-center gap-0.5 justify-end">
                    <ButtonIcon
                      v-if="msg.role === 'user' && index === lastUserMessageIndex && !isGenerating"
                      icon="mdi:pencil-outline"
                      class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
                      tooltip-content="编辑"
                      @click.stop="handleEditMessage(index)"
                    />
                    <ButtonIcon
                      v-if="msg.role === 'assistant' && msg.isError && !isGenerating"
                      icon="mdi:refresh"
                      class="!h-28px !w-28px text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400"
                      tooltip-content="重试"
                      @click.stop="handleRetryMessage(index)"
                    />
                    <ButtonIcon
                      icon="mdi:content-copy"
                      class="!h-28px !w-28px text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
                      tooltip-content="复制"
                      @click.stop="copyToClipboard(msg.content)"
                    />
                    <ButtonIcon
                      v-if="enableVocabulary && msg.role === 'assistant'"
                      icon="mdi:star-outline"
                      class="!h-28px !w-28px text-gray-400 hover:text-amber-500 dark:text-gray-500 dark:hover:text-amber-400"
                      tooltip-content="添加到生词本"
                      @click.stop="openVocabModal()"
                    />
                    <ButtonIcon
                      v-if="msg.role === 'assistant'"
                      icon="mdi:notebook-edit-outline"
                      class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
                      tooltip-content="添加笔记"
                      @click.stop="openNoteModal(msg.content)"
                    />
                  </div>

                  <div
                    v-if="enableVocabulary && msg.suggestions?.length"
                    class="flex flex-wrap gap-2 mt-2"
                  >
                    <span class="text-xs text-gray-400 self-center">智能建议:</span>
                    <NTag
                      v-for="(vocab, vocabIndex) in msg.suggestions"
                      :key="vocabIndex"
                      size="small"
                      round
                      type="info"
                      check-strategy="child"
                      class="cursor-pointer hover:shadow-sm transition-shadow"
                      @click="handleApplySuggestion(vocab)"
                    >
                      <template #icon>
                        <div class="i-mdi:plus" />
                      </template>
                      {{ vocab.word }}
                    </NTag>
                  </div>

                  <div
                    v-if="hasAuth('ai:course:edit') && msg.expressions?.length"
                    class="flex flex-wrap gap-2 mt-2"
                  >
                    <span class="text-xs text-gray-400 self-center">添加到课程:</span>
                    <NTag
                      v-for="(expr, exprIndex) in msg.expressions"
                      :key="exprIndex"
                      size="small"
                      round
                      type="success"
                      check-strategy="child"
                      class="cursor-pointer hover:shadow-sm transition-shadow"
                      @click="handleAddExpression(expr)"
                    >
                      <template #icon>
                        <div class="i-mdi:plus" />
                      </template>
                      {{ expr.english.slice(0, 30) }}{{ expr.english.length > 30 ? '...' : '' }}
                    </NTag>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </NScrollbar>

        <!-- Scroll shortcuts -->
        <div class="scroll-shortcuts">
          <NButton
            quaternary
            circle
            size="small"
            class="scroll-btn"
            @click="scrollToTop('smooth')"
          >
            <template #icon>
              <SvgIcon icon="mdi:chevron-up" />
            </template>
          </NButton>
          <NButton
            v-if="hasAnyThinkingContent"
            quaternary
            circle
            size="small"
            class="scroll-btn"
            @click="toggleAllThinking"
          >
            <template #icon>
              <SvgIcon :icon="allThinkingExpanded ? 'mdi:chevron-double-up' : 'mdi:chevron-double-down'" />
            </template>
          </NButton>
          <NButton
            quaternary
            circle
            size="small"
            class="scroll-btn"
            @click="scrollToBottom('smooth')"
          >
            <template #icon>
              <SvgIcon icon="mdi:chevron-down" />
            </template>
          </NButton>
        </div>
      </div>

      <!-- Input area -->
      <div class="input-wrapper" :class="appStore.isMobile ? 'mobile' : 'desktop'">
        <div class="input-container">
          <!-- Desktop: single row layout -->
          <div v-if="!appStore.isMobile" class="input-main">
            <!-- Left: Model Selector -->
            <div v-if="!isOrchestration" class="model-selector">
              <SvgIcon icon="mdi:cpu-chip" class="model-icon" />
              <NSelect
                v-model:value="selectedModel"
                :options="modelOptions"
                size="tiny"
                :consistent-menu-width="false"
                :bordered="false"
                class="model-select"
              />
            </div>

            <!-- Middle: Text Input -->
            <NInput
              v-model:value="inputMessage"
              type="textarea"
              :autosize="{ minRows: 1, maxRows: 6 }"
              :placeholder="inputPlaceholder"
              :bordered="false"
              class="input-textarea"
              @keydown="handleEnter"
            />

            <!-- Right Actions -->
            <div class="right-actions">
              <Transition name="scale">
                <NButton
                  v-if="inputMessage.trim() && !isGenerating"
                  quaternary
                  circle
                  size="small"
                  class="clear-btn"
                  @click="inputMessage = ''"
                >
                  <template #icon>
                    <SvgIcon icon="mdi:trash-can-outline" />
                  </template>
                </NButton>
              </Transition>
              <Transition name="scale" mode="out-in">
                <NButton
                  v-if="isGenerating && abortController"
                  key="stop"
                  quaternary
                  circle
                  size="small"
                  class="stop-btn"
                  title="停止生成"
                  @click="handleStopGeneration"
                >
                  <template #icon>
                    <SvgIcon icon="mdi:stop-circle-outline" />
                  </template>
                </NButton>
                <NButton
                  v-else
                  key="send"
                  v-show="inputMessage.trim()"
                  type="primary"
                  size="small"
                  class="send-btn send-btn-active"
                  @click="sendMessage"
                >
                  <template #icon>
                    <SvgIcon icon="mdi:arrow-up" class="send-icon" />
                  </template>
                </NButton>
              </Transition>
            </div>
          </div>

          <!-- Mobile: two row layout -->
          <template v-else>
            <div class="input-row-textarea">
              <NInput
                v-model:value="inputMessage"
                type="textarea"
                :autosize="{ minRows: 1, maxRows: 4 }"
                placeholder=""
                :bordered="false"
                class="input-textarea"
                @keydown="handleEnter"
              />
            </div>
            <div class="input-row-actions">
              <div v-if="!isOrchestration" class="model-selector model-selector-mobile">
                <NSelect
                  v-model:value="selectedModel"
                  :options="modelOptions"
                  size="tiny"
                  :consistent-menu-width="false"
                  :bordered="false"
                  class="model-select"
                />
              </div>
              <div class="right-actions">
                <Transition name="scale">
                  <NButton
                    v-if="inputMessage.trim() && !isGenerating"
                    quaternary
                    circle
                    size="tiny"
                    class="clear-btn"
                    @click="inputMessage = ''"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:trash-can-outline" />
                    </template>
                  </NButton>
                </Transition>
                <Transition name="scale" mode="out-in">
                  <NButton
                    v-if="isGenerating && abortController"
                    key="stop"
                    quaternary
                    circle
                    size="tiny"
                    class="stop-btn"
                    title="停止生成"
                    @click="handleStopGeneration"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:stop-circle-outline" />
                    </template>
                  </NButton>
                  <NButton
                    v-else
                    key="send"
                    v-show="inputMessage.trim()"
                    type="primary"
                    size="tiny"
                    class="send-btn-mobile send-btn-active"
                    @click="sendMessage"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:arrow-up" class="send-icon" />
                    </template>
                  </NButton>
                </Transition>
              </div>
            </div>
          </template>
        </div>
      </div>
    </NCard>

    <!-- 生词本/笔记/课程包弹窗 (子组件, 表单逻辑内聚) -->
    <VocabModal v-if="enableVocabulary" ref="vocabModalRef" :play="handlePlay" />
    <NoteModal ref="noteModalRef" :render-markdown="renderMarkdown" />
    <CourseItemModal ref="courseModalRef" />

    <NModal
      v-model:show="showTitleModal"
      preset="dialog"
      title="编辑标题"
      positive-text="保存"
      negative-text="取消"
      @positive-click="handleSaveTitle"
    >
      <NInput
        v-model:value="editTitle"
        placeholder="请输入标题"
        maxlength="50"
        show-count
      />
    </NModal>

    <NDrawer
      v-model:show="showPromptEditor"
      :width="appStore.isMobile ? '85vw' : 600"
      placement="right"
    >
      <NDrawerContent
        :title="`设置 - ${routeTitleMap[route.name as string] || 'AI 助手'}`"
        closable
        body-content-style="padding: 0; display: flex; flex-direction: column; height: 100%;"
      >
        <PromptEditor
          :agent-id="props.agentId"
          :module-name="routeTitleMap[route.name as string]"
          :default-prompt="props.systemPrompt"
          @updated="refreshPrompt"
        />
      </NDrawerContent>
    </NDrawer>

    <NModal
      v-model:show="showToolApprovalModal"
      preset="card"
      title="工具调用确认"
      :style="{ width: appStore.isMobile ? '95vw' : '500px' }"
      :segmented="{ content: 'soft', footer: 'soft' }"
      :mask-closable="false"
      :closable="false"
    >
      <div class="space-y-4">
        <div class="flex items-center gap-2 text-amber-500">
          <SvgIcon icon="mdi:alert-circle-outline" class="text-xl" />
          <span class="font-medium">AI 请求调用工具</span>
        </div>
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-gray-800">
          <div class="mb-2">
            <span class="text-sm text-gray-500">工具名称：</span>
            <span class="font-medium">{{ toolApprovalInfo?.toolName }}</span>
          </div>
          <div>
            <span class="text-sm text-gray-500">调用参数：</span>
            <pre class="mt-1 overflow-x-auto text-sm">{{ toolApprovalInfo?.arguments }}</pre>
          </div>
        </div>
        <p class="text-sm text-gray-500">是否允许该工具执行？</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="handleToolApproval(false, '用户拒绝')">拒绝</NButton>
          <NButton type="primary" @click="handleToolApproval(true)">允许</NButton>
        </div>
      </template>
    </NModal>

    <!-- 编排对话的工具审批弹窗 -->
    <NModal
      :show="!!orchApprovalInfo"
      preset="card"
      title="工具执行审批"
      :style="{ width: appStore.isMobile ? '95vw' : '500px' }"
      :segmented="{ content: 'soft', footer: 'soft' }"
      :mask-closable="false"
      :closable="false"
    >
      <div class="space-y-4">
        <div class="flex items-center gap-2 text-amber-500">
          <SvgIcon icon="mdi:alert-circle-outline" class="text-xl" />
          <span class="font-medium">编排请求调用工具</span>
        </div>
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-gray-800">
          <div class="mb-2">
            <span class="text-sm text-gray-500">工具名称：</span>
            <span class="font-medium">{{ orchApprovalInfo?.tool }}</span>
          </div>
          <div>
            <span class="text-sm text-gray-500">调用参数：</span>
            <pre class="mt-1 overflow-x-auto text-sm">{{ orchApprovalInfo?.arguments }}</pre>
          </div>
        </div>
        <p class="text-sm text-gray-500">
          是否允许该工具执行？批准后立即执行；拒绝会把原因返回给模型继续作答；等待超过 2 分钟未决定将自动拒绝。
        </p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton :disabled="orchApproving" @click="handleOrchApproval(false)">拒绝</NButton>
          <NButton type="primary" :loading="orchApproving" @click="handleOrchApproval(true)">批准执行</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
:deep(.msg-content p) {
  margin: 0;
}
:deep(.msg-content p + p) {
  margin-top: 0.5em;
}

/* 思考过程折叠区域 */
.thinking-wrapper {
  border-left: 3px solid #d1d5db;
  border-radius: 8px;
  background: rgba(249, 250, 251, 0.8);
  overflow: hidden;
}
.dark .thinking-wrapper {
  border-left-color: #4b5563;
  background: rgba(55, 65, 81, 0.5);
}
.thinking-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 12px;
  font-size: 13px;
  color: #6b7280;
  background: transparent;
  border: none;
  cursor: pointer;
  transition: color 0.2s;
}
.thinking-toggle:hover {
  color: #374151;
}
.dark .thinking-toggle {
  color: #9ca3af;
}
.dark .thinking-toggle:hover {
  color: #d1d5db;
}
.thinking-toggle-icon {
  display: inline-block;
  font-size: 10px;
  transition: transform 0.2s ease;
}
.thinking-toggle-icon.expanded {
  transform: rotate(90deg);
}
.thinking-body {
  padding: 0 12px 8px;
}
.thinking-content {
  font-size: 13px;
  color: #6b7280;
  line-height: 1.6;
  white-space: pre-wrap;
}
.dark .thinking-content {
  color: #9ca3af;
}
:deep(.thinking-content p) {
  margin: 0;
}
:deep(.thinking-content p + p) {
  margin-top: 0.5em;
}
/* 展开/收起动画 */
.thinking-expand-enter-active,
.thinking-expand-leave-active {
  transition: all 0.2s ease;
  overflow: hidden;
}
.thinking-expand-enter-from,
.thinking-expand-leave-to {
  opacity: 0;
  max-height: 0;
}
.thinking-expand-enter-to,
.thinking-expand-leave-from {
  opacity: 1;
  max-height: 500px;
}

/* ============ Input Area ============ */
.input-wrapper {
  position: relative;
  flex-shrink: 0;
  width: 100%;
}
.input-wrapper.desktop {
  padding: 16px 16px 20px;
}
.input-wrapper.mobile {
  padding: 6px 6px 12px;
}

.input-container {
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid #e5e7eb;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.05), 0 4px 12px -2px rgba(0, 0, 0, 0.02);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  display: flex;
  flex-direction: column;
}
.dark .input-container {
  background: rgba(26, 26, 46, 0.85);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-color: rgba(255, 255, 255, 0.08);
  box-shadow: 0 10px 30px -10px rgba(0, 0, 0, 0.3), 0 4px 12px -2px rgba(0, 0, 0, 0.2);
}
.input-container:focus-within {
  border-color: #6366f1;
  box-shadow: 0 10px 25px -3px rgba(99, 102, 241, 0.12),
    0 4px 12px -2px rgba(99, 102, 241, 0.06);
  transform: translateY(-2px);
}
.dark .input-container:focus-within {
  border-color: rgba(99, 102, 241, 0.8);
  box-shadow: 0 10px 30px -5px rgba(99, 102, 241, 0.2),
    0 4px 12px -2px rgba(99, 102, 241, 0.12);
}

.input-main {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px 6px 12px;
  min-height: 40px;
}

/* Mobile two-row layout */
.input-row-textarea {
  padding: 8px 10px 4px;
}
.input-row-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px 8px;
  gap: 8px;
}

.right-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  margin-bottom: 2px;
}

.model-selector {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 4px;
  border-radius: 8px;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  flex-shrink: 0;
  overflow: visible;
}
.model-selector-mobile {
  flex: 1;
  min-width: 100px;
  max-width: 70%;
}
.model-icon {
  font-size: 14px;
  color: #6366f1;
  transition: transform 0.3s ease;
}
.model-selector:hover .model-icon {
  transform: rotate(30deg);
}

.send-btn-mobile {
  border-radius: 50% !important;
  width: 28px;
  height: 28px;
  padding: 0 !important;
  flex-shrink: 0;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%) !important;
  color: #ffffff !important;
  border: none !important;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

:deep(.model-select .n-base-selection),
:deep(.model-select .n-base-selection-input),
:deep(.model-select .n-base-selection-overlay),
:deep(.model-select .n-base-selection-tags),
:deep(.model-select .n-base-suffix) {
  background: transparent !important;
  box-shadow: none !important;
}
:deep(.model-select .n-base-selection) {
  border: none !important;
  min-height: 22px !important;
  width: auto !important;
  min-width: 80px !important;
}
:deep(.model-select .n-base-selection .n-base-selection-label) {
  font-size: 12px !important;
  font-weight: 500;
  color: #4b5563;
  padding-left: 6px !important;
  padding-right: 18px !important;
  white-space: nowrap;
}
.dark :deep(.model-select .n-base-selection .n-base-selection-label) {
  color: #c0c4cc;
}

.clear-btn {
  color: #9ca3af;
  transition: all 0.2s;
  width: 28px;
  height: 28px;
}
.clear-btn:hover {
  color: #ef4444;
  background-color: rgba(239, 68, 68, 0.08) !important;
}
.dark .clear-btn:hover {
  color: #f87171;
  background-color: rgba(248, 113, 113, 0.12) !important;
}

/* 停止生成按钮 */
.stop-btn {
  color: #6b7280;
  transition: all 0.2s;
}
.stop-btn:hover {
  color: #ef4444;
  background-color: rgba(239, 68, 68, 0.08) !important;
}
.dark .stop-btn {
  color: #9ca3af;
}
.dark .stop-btn:hover {
  color: #f87171;
  background-color: rgba(248, 113, 113, 0.12) !important;
}

:deep(.input-textarea .n-input__textarea-el) {
  padding: 6px 8px !important;
  line-height: 1.6 !important;
  caret-color: #6366f1;
  font-size: 14px;
}
:deep(.input-textarea .n-input__placeholder) {
  padding: 6px 8px !important;
  color: #9ca3af;
}
.dark :deep(.input-textarea .n-input__placeholder) {
  color: #52526b;
}

.send-btn {
  border-radius: 50% !important;
  width: 32px;
  height: 32px;
  padding: 0 !important;
  flex-shrink: 0;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%) !important;
  color: #ffffff !important;
  border: none !important;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
.send-btn:hover {
  transform: scale(1.1) translateY(-1px);
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.45);
}
.send-btn:active {
  transform: scale(0.95) translateY(0);
}

.send-icon {
  font-size: 16px;
  transition: transform 0.2s ease;
}
.send-btn.send-btn-active:hover .send-icon {
  transform: translateY(-1px);
}


.scale-enter-active,
.scale-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
.scale-enter-from,
.scale-leave-to {
  opacity: 0;
  transform: scale(0.8);
}

.scroll-shortcuts {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  z-index: 10;
  pointer-events: auto;
}
.scroll-btn {
  width: 32px;
  height: 32px;
  background: rgba(255, 255, 255, 0.9) !important;
  border: 1px solid #e5e7eb !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  color: #6b7280;
  transition: all 0.2s;
  pointer-events: auto;
}
.scroll-btn:hover {
  background: #ffffff !important;
  color: #6366f1;
  border-color: #6366f1 !important;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.2);
}
.dark .scroll-btn {
  background: rgba(30, 30, 46, 0.9) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: #9ca3af;
}
.dark .scroll-btn:hover {
  background: rgba(30, 30, 46, 1) !important;
  color: #818cf8;
  border-color: #818cf8 !important;
}

.thinking-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #60a5fa;
  animation: thinking-bounce 1.4s infinite ease-in-out both;
}

.dark .thinking-dot {
  background-color: #93c5fd;
}

@keyframes thinking-bounce {
  0%,
  80%,
  100% {
    transform: scale(0);
    opacity: 0.5;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>
