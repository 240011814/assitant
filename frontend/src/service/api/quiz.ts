import { request } from '../request';

/** 出题练习题型 */
export type QuizType = 'word2def' | 'def2word' | 'cloze' | 'confusing' | 'context' | 'spell' | 'translate';

/** 服务端题目 (答案剥离, 判分结果随下发) */
export interface QuizQuestion {
  id: string;
  type: QuizType;
  wordId?: number;
  courseId?: number;
  prompt: string;
  hint?: string;
  options?: string[];
  word?: string;
  done: boolean;
  /** 判分结果 (Done 后有效) */
  correct: boolean;
}

export interface QuizSession {
  id: number;
  total: number;
  answered: number;
  correct: number;
  status: 'active' | 'finished' | 'abandoned';
}

export interface QuizAnswerResult {
  questionId: string;
  correct: boolean;
  answer: string;
  recorded: boolean;
  /** 答对是否消了一条对应错题 */
  decayed?: boolean;
  /** 答错讲解 (词义/例句/易混对比) */
  explanation?: string;
}

/** 创建练习会话 */
export function fetchCreateQuizSession(data: { size?: number; types?: QuizType[]; source?: 'all' | 'due' | 'errorbook' }) {
  return request<{ session: QuizSession; questions: QuizQuestion[] }>({
    url: '/api/vocabulary/quiz/sessions',
    method: 'post',
    data
  });
}

/** 获取练习会话 */
export function fetchGetQuizSession(id: number) {
  return request<{ session: QuizSession; questions: QuizQuestion[] }>({
    url: `/api/vocabulary/quiz/sessions/${id}`,
    method: 'get'
  });
}

/** 最近一个进行中的会话 (48h 内, 供"继续练习"); 无则 data 为 null */
export function fetchGetActiveQuizSession() {
  return request<{ session: QuizSession; questions: QuizQuestion[] } | null>({
    url: '/api/vocabulary/quiz/sessions/active',
    method: 'get'
  });
}

/** 提交单题答案 (服务端判分, 答错自动进错题本/答对消错, 结果回流 FSRS) */
export function fetchSubmitQuizAnswer(id: number, data: { questionId: string; answer: string; durationMs?: number }) {
  return request<{ result: QuizAnswerResult; session: QuizSession; questions: QuizQuestion[] }>({
    url: `/api/vocabulary/quiz/sessions/${id}/answer`,
    method: 'post',
    data
  });
}

/** 造句批改 (LLM) */
export function fetchSentencePractice(data: { wordId: number; sentence: string }) {
  return request<{ score: number; correct: boolean; grammar: string[]; improved: string; comment: string; word: string }>({
    url: '/api/vocabulary/quiz/sentence-practice',
    method: 'post',
    data
  });
}
