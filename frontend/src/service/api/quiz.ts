import { request } from '../request';

/** 出题练习题型 */
export type QuizType = 'word2def' | 'def2word' | 'cloze' | 'confusing' | 'spell' | 'translate';

/** 服务端题目 (答案剥离) */
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
}

export interface QuizSession {
  id: number;
  total: number;
  answered: number;
  correct: number;
  status: 'active' | 'finished';
}

export interface QuizAnswerResult {
  questionId: string;
  correct: boolean;
  answer: string;
  recorded: boolean;
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

/** 提交单题答案 (服务端判分, 答错自动进错题本) */
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
