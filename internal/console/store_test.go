package console

import (
	"bytes"
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

func TestApprovalRules(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.AddRule("ls"); err != nil {
		t.Fatal(err)
	}
	if err = s.AddRule("cat src/"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.RuleRows()
	if err != nil || len(rows) != 2 || rows[0].Pattern != "ls" || rows[1].Pattern != "cat src/" {
		t.Fatalf("rules %+v err %v", rows, err)
	}
	if err = s.DeleteRule(rows[1].ID); err != nil {
		t.Fatal(err)
	}
	rows, err = s.RuleRows()
	if err != nil || len(rows) != 1 || rows[0].Pattern != "ls" {
		t.Fatalf("after delete %+v err %v", rows, err)
	}
	if err = s.AddRule("  "); err == nil {
		t.Fatal("accepted blank rule")
	}
}

func TestAPIKeysRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	s.SetCipherSeed(seed)
	if err = s.SaveAPIKey("openrouter", "sk-secret-value"); err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := s.db.QueryRow("SELECT ciphertext FROM api_keys").Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("sk-secret-value")) {
		t.Fatal("plaintext stored in ciphertext column")
	}
	rows, err := s.APIKeyRows()
	if err != nil || len(rows) != 1 || rows[0].Name != "openrouter" {
		t.Fatalf("key rows %+v err %v", rows, err)
	}
	values, err := s.APIKeyValues()
	if err != nil || len(values) != 1 || values[0].Name != "openrouter" || values[0].Value != "sk-secret-value" {
		t.Fatalf("values %+v err %v", values, err)
	}
	// 錯誤密碼必須明確報錯，不可靜默啟動缺少金鑰的程序。
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var wrong [32]byte
	wrong[0] = 9
	s2.SetCipherSeed(wrong)
	values2, err := s2.APIKeyValues()
	if err == nil || len(values2) != 0 {
		t.Fatal("wrong seed must return an error and no values")
	}
	s2.SetCipherSeed(seed)
	if values, err := s2.APIKeyValues(); err != nil || len(values) != 1 || values[0].Value != "sk-secret-value" {
		t.Fatal("key did not survive reopen")
	}
	if _, err := s.db.Exec("UPDATE api_keys SET nonce=x'01'"); err != nil {
		t.Fatal(err)
	}
	if values, err := s.APIKeyValues(); err == nil || len(values) != 0 {
		t.Fatal("corrupt nonce must fail closed")
	}
	if err = s.DeleteAPIKey(rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if rows, _ = s.APIKeyRows(); len(rows) != 0 {
		t.Fatal("key not deleted")
	}
}
