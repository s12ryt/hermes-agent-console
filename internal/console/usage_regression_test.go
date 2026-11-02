package console

import "testing"

func TestUsageSnapshotsDoNotDoubleCount(t *testing.T) {
	s := testServer(t).store
	for _, calls := range []int64{1, 2, 2, 1} {
		if err := s.RecordUsage(Usage{SessionID: "same", Calls: calls, Input: float64(calls * 10), Output: float64(calls * 5), Total: float64(calls * 15)}); err != nil {
			t.Fatal(err)
		}
	}
	total, err := s.UsageTotals()
	if err != nil || total.Calls != 2 || total.Total != 30 {
		t.Fatalf("snapshots counted repeatedly: %+v %v", total, err)
	}
}

func TestUsageTotalsSurviveHistoryPruning(t *testing.T) {
	s := testServer(t).store
	if err := s.RecordUsage(Usage{SessionID: "old", Calls: 3, Total: 90}); err != nil {
		t.Fatal(err)
	}
	// 模擬有限長度活動歷史清理；累計資料必須獨立保留。
	if _, err := s.db.Exec("DELETE FROM usage"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage(Usage{SessionID: "new", Calls: 1, Total: 10}); err != nil {
		t.Fatal(err)
	}
	total, err := s.UsageTotals()
	if err != nil || total.Calls != 4 || total.Total != 100 {
		t.Fatalf("pruning lost totals: %+v %v", total, err)
	}
}
