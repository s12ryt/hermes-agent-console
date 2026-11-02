package console

import (
	"strings"
	"unicode/utf8"
)

type SearchHit struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

func (s *Store) indexConversation(session, text string) error {
	if session == "" || len(text) > 4<<20 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM conversation_search WHERE session_id=?", session); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO conversation_search(session_id,text) VALUES(?,?)", session, text); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) searchConversations(query string) ([]SearchHit, error) {
	out := []SearchHit{}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 256 {
		return out, nil
	}
	sqlQuery := "SELECT session_id,substr(text,1,1000) FROM conversation_search WHERE conversation_search MATCH ? LIMIT 50"
	param := "\"" + strings.ReplaceAll(query, "\"", "\"\"") + "\""
	if utf8.RuneCountInString(query) < 3 {
		sqlQuery = "SELECT session_id,substr(text,1,1000) FROM conversation_search WHERE instr(lower(text),lower(?))>0 LIMIT 50"
		param = query
	}
	rows, err := s.db.Query(sqlQuery, param)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var hit SearchHit
		if err = rows.Scan(&hit.SessionID, &hit.Text); err != nil {
			return nil, err
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}
