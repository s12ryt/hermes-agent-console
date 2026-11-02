package console

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupRestoreAPI(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	if w := request(app, "GET", "/api/backup", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if err := app.store.AddRule("ls"); err != nil {
		t.Fatal(err)
	}
	w := request(app, "GET", "/api/backup", "", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "approval_rules") {
		t.Fatalf("backup %d %s", w.Code, w.Body)
	}
	saved := w.Body.String()
	if w = request(app, "POST", "/api/restore", `{"version":99}`, cookie, ""); w.Code != 400 {
		t.Fatal("invalid backup accepted")
	}
	rules, _ := app.store.RuleRows()
	app.store.DeleteRule(rules[0].ID)
	if w = request(app, "POST", "/api/restore", saved, cookie, ""); w.Code != 200 {
		t.Fatalf("restore %d %s", w.Code, w.Body)
	}
	rules, _ = app.store.RuleRows()
	if len(rules) != 1 || rules[0].Pattern != "ls" {
		t.Fatal("rules not restored")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(saved), &document); err != nil {
		t.Fatal(err)
	}
	document["approval_rules"] = []map[string]any{{"pattern": ""}}
	raw, _ := json.Marshal(document)
	if w = request(app, "POST", "/api/restore", string(raw), cookie, ""); w.Code != 400 {
		t.Fatal("bad rule accepted")
	}
	rules, _ = app.store.RuleRows()
	if len(rules) != 1 {
		t.Fatal("failed restore altered state")
	}
}
