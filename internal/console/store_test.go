package console

import (
	"path/filepath"
	"testing"
)

func TestSettingsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "console.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	root := t.TempDir()
	want := Settings{Python: filepath.Join(root, "python"), Source: root, Workspace: filepath.Join(root, "work"), Home: filepath.Join(root, "home")}
	if err = s.SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Settings()
	if err != nil || got != want {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
}

func TestSettingsRejectRelativePaths(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.SaveSettings(Settings{Python: "python", Source: "relative"}); err == nil {
		t.Fatal("accepted relative source")
	}
}

func TestUsagePersistence(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecordUsage(Usage{SessionID: "a", Calls: 2, Input: 120.5, Output: 80, Total: 200.5}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordUsage(Usage{SessionID: "b", Calls: 1, Input: 10, Output: 5, Total: 15}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.UsageRows()
	if err != nil || len(rows) != 2 || rows[0].SessionID != "b" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
	tot, err := s.UsageTotals()
	if err != nil || tot.Calls != 3 || tot.Input != 130.5 || tot.Output != 85 || tot.Total != 215.5 {
		t.Fatalf("totals %+v err %v", tot, err)
	}
}
