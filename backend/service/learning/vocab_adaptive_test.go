package learning

import (
	"testing"

	"backend/model"
)

func adaptiveTestPool() *adaptivePool {
	pool := &adaptivePool{Questions: []model.VocabTestQuestion{
		{ID: "a1w", Band: "A1", Word: "a1w", Options: []string{"对", "错1", "错2", "错3"}, Answer: 0},
		{ID: "a2w", Band: "A2", Word: "a2w", Options: []string{"对", "错1", "错2", "错3"}, Answer: 1},
		{ID: "b1w", Band: "B1", Word: "b1w", Options: []string{"错1", "对", "错2", "错3"}, Answer: 1},
		{ID: "b2w", Band: "B2", Word: "b2w", Options: []string{"错1", "错2", "对", "错3"}, Answer: 2},
		{ID: "c1w", Band: "C1", Word: "c1w", Options: []string{"对", "错1", "错2", "错3"}, Answer: 0},
		{ID: "c2w", Band: "C2", Word: "c2w", Options: []string{"错1", "对", "错2", "错3"}, Answer: 1},
	}}
	return pool
}

func ans(id string, choice int) AdaptiveRoundAnswer {
	return AdaptiveRoundAnswer{QuestionID: id, Choice: choice}
}

// TestVocabReplayPath 二分重放: 通过档抬 lo / 失败档压 hi, 区间收敛到相邻档
func TestVocabReplayPath(t *testing.T) {
	pool := adaptiveTestPool()
	cases := []struct {
		name     string
		rounds   [][]AdaptiveRoundAnswer
		wantLo   int
		wantHi   int
		wantQ    int
		wantErr  bool
		converge bool // hi-lo<=1 即收敛
	}{
		{
			name:   "B1 全对 → lo=B1, hi 未定",
			rounds: [][]AdaptiveRoundAnswer{{ans("b1w", 1), ans("a1w", 0), ans("a2w", 1), ans("b1w", 1)}},
			// 注意: b1w 出现两次 (同题重复作答是客户端异常, 这里只验证重放数学)
			wantLo: 2, wantHi: 6, wantQ: 4,
		},
		{
			name:   "B1 失败 → hi=B1, lo 未定 (A1 尚未探测, 不收敛)",
			rounds: [][]AdaptiveRoundAnswer{{ans("b1w", 0), ans("b1w", 0), ans("b1w", 0), ans("b1w", 0)}},
			wantLo: -1, wantHi: 2, wantQ: 4,
		},
		{
			name:   "B1 过 C1 挂 → [B1, C1) 中点探 B2 (尚未收敛)",
			rounds: [][]AdaptiveRoundAnswer{{ans("b1w", 1), ans("b1w", 1), ans("b1w", 1), ans("b1w", 1)}, {ans("c1w", 2), ans("c1w", 2), ans("c1w", 2), ans("c1w", 2)}},
			wantLo: 2, wantHi: 4, wantQ: 8,
		},
		{
			name:   "一路通关到 C2 → lo=C2",
			rounds: [][]AdaptiveRoundAnswer{{ans("b1w", 1), ans("b1w", 1), ans("b1w", 1), ans("b1w", 1)}, {ans("c1w", 0), ans("c1w", 0), ans("c1w", 0), ans("c1w", 0)}, {ans("c2w", 1), ans("c2w", 1), ans("c2w", 1), ans("c2w", 1)}},
			wantLo: 5, wantHi: 6, wantQ: 12, converge: true,
		},
		{
			name:    "题号不在题池 → 报错",
			rounds:  [][]AdaptiveRoundAnswer{{ans("hacker", 0)}},
			wantErr: true,
		},
		{
			name:    "选项下标越界 → 报错",
			rounds:  [][]AdaptiveRoundAnswer{{ans("b1w", 9)}},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi, bands, totalQ, err := vocabReplayPath(pool, tc.rounds)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lo != tc.wantLo || hi != tc.wantHi {
				t.Fatalf("want lo=%d hi=%d, got lo=%d hi=%d", tc.wantLo, tc.wantHi, lo, hi)
			}
			if totalQ != tc.wantQ {
				t.Fatalf("want %d answered, got %d", tc.wantQ, totalQ)
			}
			if tc.converge && hi-lo > 1 {
				t.Fatalf("want converged (hi-lo<=1), got gap=%d", hi-lo)
			}
			if tc.wantLo >= 0 {
				st := bands[vocabBandOrder[tc.wantLo]]
				if st.Asked == 0 {
					t.Fatalf("band %s stat missing", vocabBandOrder[tc.wantLo])
				}
			}
		})
	}
}

// TestVocabProbePath 首轮探针必须是 B1 (区间 [-1,6] 的中点), 通关路径的探针序列单调上移
func TestVocabProbePath(t *testing.T) {
	lo, hi := -1, 6
	mid := (lo + hi) / 2
	if vocabBandOrder[mid] != "B1" {
		t.Fatalf("first probe should be B1, got %s", vocabBandOrder[mid])
	}
	lo = mid
	mid = (lo + hi) / 2
	if vocabBandOrder[mid] != "C1" {
		t.Fatalf("second probe after B1 pass should be C1, got %s", vocabBandOrder[mid])
	}
	hi = mid
	mid = (lo + hi) / 2
	if vocabBandOrder[mid] != "B2" {
		t.Fatalf("probe after C1 fail should be B2, got %s", vocabBandOrder[mid])
	}
}
