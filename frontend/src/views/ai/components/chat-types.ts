/** 聊天气泡共享类型 (training-chat 与 message-bubble 使用) */

export interface VocabSuggestion {
  word?: string;
  phonetic?: string;
  definition?: string;
  example?: string;
  confusingWords?: string;
}

export interface ExpressionSuggestion {
  english: string;
  chinese: string;
}

export interface TokenUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface ChatMessage {
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
