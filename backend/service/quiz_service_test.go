package service

import (
	"encoding/json"
	"testing"

	"backend/model"
)

// TestQuizQuestionAnswerPersists 答案必须随会话 JSON 落库 (曾因 json:"-" 导致
// 存库时被剥掉, 回读判分全部判错); 剥离只允许发生在返回前端的时刻
func TestQuizQuestionAnswerPersists(t *testing.T) {
	q := model.QuizQuestion{
		ID:     "q1",
		Type:   "def2word",
		Prompt: "押金",
		Answer: "security deposit",
	}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back model.QuizQuestion
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Answer != q.Answer {
		t.Fatalf("答案落库丢失: got %q want %q", back.Answer, q.Answer)
	}
}
