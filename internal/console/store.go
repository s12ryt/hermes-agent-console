package console

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"time"
)

type Settings struct {
	Python    string `json:"python"`
	Source    string `json:"source"`
	Workspace string `json:"workspace"`
	Home      string `json:"home"`
}
type Store struct{ db *sql.DB }
type Log struct {
	ID     int64  `json:"id"`
	Time   string `json:"time"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL); CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY, time TEXT NOT NULL, kind TEXT NOT NULL, detail TEXT NOT NULL); CREATE TABLE IF NOT EXISTS usage (id INTEGER PRIMARY KEY, time TEXT NOT NULL, session_id TEXT NOT NULL, calls INTEGER NOT NULL, input REAL NOT NULL, output REAL NOT NULL, total REAL NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Settings() (Settings, error) {
	var raw string
	v := Settings{Python: "python"}
	err := s.db.QueryRow("SELECT value FROM settings WHERE id=1").Scan(&raw)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(raw), &v)
	return v, err
}
func (s *Store) SaveSettings(v Settings) error {
	if strings.TrimSpace(v.Python) == "" || strings.ContainsAny(v.Python, "\r\n\x00") {
		return fmt.Errorf("Python 執行檔不可空白")
	}
	for _, p := range []string{v.Source, v.Workspace, v.Home} {
		if p != "" && !filepath.IsAbs(p) {
			return fmt.Errorf("目錄必須使用絕對路徑")
		}
	}
	if v.Source == "" {
		return fmt.Errorf("請填寫 Hermes 原始碼目錄")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO settings(id,value) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", string(raw))
	return err
}
func (s *Store) AddLog(kind, detail string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO logs(time,kind,detail) VALUES(?,?,?)", time.Now().UTC().Format(time.RFC3339), kind, detail); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM logs WHERE id <= (SELECT MAX(id)-1000 FROM logs)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Logs() ([]Log, error) {
	rows, err := s.db.Query("SELECT id,time,kind,detail FROM logs ORDER BY id DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Log{}
	for rows.Next() {
		var v Log
		if err = rows.Scan(&v.ID, &v.Time, &v.Kind, &v.Detail); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Usage struct {
	ID        int64   `json:"id"`
	Time      string  `json:"time"`
	SessionID string  `json:"session_id"`
	Calls     int64   `json:"calls"`
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	Total     float64 `json:"total"`
}

func (s *Store) RecordUsage(u Usage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO usage(time,session_id,calls,input,output,total) VALUES(?,?,?,?,?,?)", time.Now().UTC().Format(time.RFC3339), u.SessionID, u.Calls, u.Input, u.Output, u.Total); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM usage WHERE id <= (SELECT MAX(id)-5000 FROM usage)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) UsageRows() ([]Usage, error) {
	rows, err := s.db.Query("SELECT id,time,session_id,calls,input,output,total FROM usage ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var v Usage
		if err = rows.Scan(&v.ID, &v.Time, &v.SessionID, &v.Calls, &v.Input, &v.Output, &v.Total); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) UsageTotals() (Usage, error) {
	var v Usage
	err := s.db.QueryRow("SELECT COALESCE(SUM(calls),0),COALESCE(SUM(input),0),COALESCE(SUM(output),0),COALESCE(SUM(total),0) FROM usage").Scan(&v.Calls, &v.Input, &v.Output, &v.Total)
	return v, err
}
