package console

import "testing"

func TestRulesRejectShellOperatorsOnSaveAndRestore(t *testing.T) {
	s := testServer(t).store
	for _, pattern := range []string{"ls;whoami", "ls && pwd", "echo $(id)", "cat > out", "ls\nwhoami"} {
		if err := s.AddRule(pattern); err == nil {
			t.Errorf("saved unsafe rule %q", pattern)
		}
		if err := s.restoreBackup(backup{Version: 1, Settings: Settings{Python: "python"}, Rules: []Rule{{Pattern: pattern}}}); err == nil {
			t.Errorf("restored unsafe rule %q", pattern)
		}
	}
}
