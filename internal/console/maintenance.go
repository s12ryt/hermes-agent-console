package console

import (
	"encoding/json"
	"errors"
	"net/http"
)

type encryptedKey struct {
	Name       string `json:"name"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
	Created    string `json:"created"`
}
type backup struct {
	Version  int            `json:"version"`
	Settings Settings       `json:"settings"`
	Rules    []Rule         `json:"approval_rules"`
	Keys     []encryptedKey `json:"encrypted_keys"`
}

func (s *Store) exportBackup() (backup, error) {
	b := backup{Version: 1, Keys: []encryptedKey{}}
	var err error
	b.Settings, err = s.Settings()
	if err != nil {
		return b, err
	}
	b.Rules, err = s.RuleRows()
	if err != nil {
		return b, err
	}
	rows, err := s.db.Query("SELECT name,salt,nonce,ciphertext,created FROM api_keys ORDER BY id")
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		var k encryptedKey
		if err = rows.Scan(&k.Name, &k.Salt, &k.Nonce, &k.Ciphertext, &k.Created); err != nil {
			return b, err
		}
		b.Keys = append(b.Keys, k)
	}
	return b, rows.Err()
}
func (s *Store) restoreBackup(b backup) error {
	if b.Version != 1 || len(b.Rules) > 1000 || len(b.Keys) > 4 {
		return errors.New("不支援或過大的備份")
	}
	for _, r := range b.Rules {
		if !validApprovalRule(r.Pattern) {
			return errors.New("備份包含無效核准規則")
		}
	}
	seen := map[string]bool{}
	for _, k := range b.Keys {
		if seen[k.Name] || providerVariables[k.Name] == "" || len(k.Salt) != 16 || len(k.Nonce) != 12 || len(k.Ciphertext) > 8208 {
			return errors.New("備份金鑰無效")
		}
		seen[k.Name] = true
		aead, err := s.keyCipher(k.Salt)
		if err != nil {
			return err
		}
		plain, err := aead.Open(nil, k.Nonce, k.Ciphertext, []byte(k.Name))
		if err != nil {
			return errors.New("備份密碼不符或金鑰損壞")
		}
		clear(plain)
	}
	// Reuse the same settings validation without changing the live database.
	if b.Settings.Source != "" {
		if err := validateSettings(b.Settings); err != nil {
			return err
		}
	} else if b.Settings.Workspace != "" || b.Settings.Home != "" {
		return errors.New("備份設定不完整")
	}
	raw, err := json.Marshal(b.Settings)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM approval_rules; DELETE FROM api_keys"); err != nil {
		return err
	}
	for _, r := range b.Rules {
		if _, err = tx.Exec("INSERT INTO approval_rules(pattern,created) VALUES(?,?)", r.Pattern, r.Created); err != nil {
			return err
		}
	}
	for _, k := range b.Keys {
		if _, err = tx.Exec("INSERT INTO api_keys(name,salt,nonce,ciphertext,created) VALUES(?,?,?,?,?)", k.Name, k.Salt, k.Nonce, k.Ciphertext, k.Created); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO settings(id,value) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.exportBackup()
	if err != nil {
		fail(w, 500, "無法建立備份")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="hermes-console-backup.json"`)
	writeJSON(w, 200, b)
}
func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var b backup
	if !decode(w, r, &b) {
		return
	}
	if s.gateway.Status().Connected || s.installer.Status().Running {
		fail(w, 409, "請先停止 Hermes 與安裝工作")
		return
	}
	if err := s.store.restoreBackup(b); err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
