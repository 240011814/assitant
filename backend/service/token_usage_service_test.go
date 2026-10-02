package service

import (
	"testing"
	"time"
)

// TestOrchTokenBuckets 趋势分桶: 连续、对齐、各粒度键格式正确
func TestOrchTokenBuckets(t *testing.T) {
	cases := []struct {
		name        string
		granularity string
		start       time.Time
		end         time.Time
		first       string
		last        string
		count       int
	}{
		{
			name:        "day",
			granularity: "day",
			start:       time.Date(2026, 10, 1, 15, 30, 0, 0, time.Local),
			end:         time.Date(2026, 10, 3, 2, 0, 0, 0, time.Local),
			first:       "2026-10-01",
			last:        "2026-10-03",
			count:       3,
		},
		{
			name:        "hour",
			granularity: "hour",
			start:       time.Date(2026, 10, 2, 10, 20, 0, 0, time.Local),
			end:         time.Date(2026, 10, 2, 13, 10, 0, 0, time.Local),
			first:       "2026-10-02 10:00",
			last:        "2026-10-02 13:00",
			count:       4,
		},
		{
			name:        "month",
			granularity: "month",
			start:       time.Date(2026, 1, 31, 23, 0, 0, 0, time.Local),
			end:         time.Date(2026, 3, 2, 0, 0, 0, 0, time.Local),
			first:       "2026-01",
			last:        "2026-03",
			count:       3,
		},
		{
			name:        "year",
			granularity: "year",
			start:       time.Date(2025, 11, 1, 0, 0, 0, 0, time.Local),
			end:         time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local),
			first:       "2025",
			last:        "2026",
			count:       2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format, ok := granularityFormat[tc.granularity]
			if !ok {
				t.Fatalf("granularity %s missing format", tc.granularity)
			}
			buckets := orchTokenBuckets(TokenUsageStatsParams{
				Granularity: tc.granularity,
				StartTime:   tc.start,
				EndTime:     tc.end,
			}, format)
			if len(buckets) != tc.count {
				t.Fatalf("bucket count = %d, want %d: %v", len(buckets), tc.count, buckets)
			}
			if buckets[0] != tc.first || buckets[len(buckets)-1] != tc.last {
				t.Fatalf("buckets = %v, want first=%s last=%s", buckets, tc.first, tc.last)
			}
			// 连续性: 相邻桶不重复
			seen := map[string]bool{}
			for _, b := range buckets {
				if seen[b] {
					t.Fatalf("duplicate bucket %s", b)
				}
				seen[b] = true
			}
		})
	}
}

func TestOrchTokenBucketsZeroTime(t *testing.T) {
	if buckets := orchTokenBuckets(TokenUsageStatsParams{Granularity: "day"}, granularityFormat["day"]); buckets != nil {
		t.Fatalf("zero time should return nil, got %v", buckets)
	}
}

func TestMysqlFormatToGo(t *testing.T) {
	if got := mysqlFormatToGo("%Y-%m-%d %H:00"); got != "2006-01-02 15:00" {
		t.Fatalf("got %s", got)
	}
	if got := mysqlFormatToGo("%Y-%m"); got != "2006-01" {
		t.Fatalf("got %s", got)
	}
}
