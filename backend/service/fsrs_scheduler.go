package service

import (
	"time"

	"github.com/open-spaced-repetition/go-fsrs"

	"backend/model"
)

// FSRS 调度封装: 默认参数 (目标记忆保持率 0.9), 由生词记录构造卡片并计算复习后的新状态

var fsrsParams = fsrs.DefaultParam()

// masteredIntervalDays 调度间隔达到该天数视为已掌握 (毕业)
const masteredIntervalDays = 21

// fsrsCardFromWord 由生词的 FSRS 字段构造调度卡片;
// 没有复习记录的词一律按全新卡处理 (避免零值 LastReview 把 ElapsedDays 算成天文数字)
func fsrsCardFromWord(w *model.Vocabulary) fsrs.Card {
	card := fsrs.NewCard()
	if w.LastReviewedAt == nil || w.FsrsState == model.FsrsStateNew {
		return card
	}
	card.State = fsrs.State(w.FsrsState)
	card.Stability = w.FsrsStability
	card.Difficulty = w.FsrsDifficulty
	card.Reps = uint64(w.FsrsReps)
	card.Lapses = uint64(w.FsrsLapses)
	card.LastReview = *w.LastReviewedAt
	return card
}

// applyFsrsReview 计算一次复习后的新卡片状态
func applyFsrsReview(w *model.Vocabulary, rating int, now time.Time) fsrs.Card {
	card := fsrsCardFromWord(w)
	infos := fsrsParams.Repeat(card, now)
	return infos[fsrs.Rating(rating)].Card
}

// fsrsCardIsMastered 复习后是否视为已掌握: 进入 Review 态且调度间隔达到毕业天数;
// 忘记 (Again) 一律回炉取消已掌握
func fsrsCardIsMastered(next fsrs.Card, rating int) bool {
	if rating == int(fsrs.Again) {
		return false
	}
	return next.State == fsrs.Review && float64(next.ScheduledDays) >= masteredIntervalDays
}
