package console

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
)

type APIKeyInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}
type APIKeyPair struct{ Name, Value string }

var providerVariables = map[string]string{"openrouter": "OPENROUTER_API_KEY", "openai": "OPENAI_API_KEY", "anthropic": "ANTHROPIC_API_KEY", "gemini": "GEMINI_API_KEY"}

func (s *Store) SetCipherSeed(seed [32]byte) {
	s.cipherMu.Lock()
	defer s.cipherMu.Unlock()
	s.cipherSeed = &seed
}
func (s *Store) keyCipher(salt []byte) (cipher.AEAD, error) {
	s.cipherMu.RLock()
	if s.cipherSeed == nil {
		s.cipherMu.RUnlock()
		return nil, errors.New("金鑰加密尚未初始化")
	}
	seed := *s.cipherSeed
	s.cipherMu.RUnlock()
	key, err := pbkdf2.Key(sha256.New, string(seed[:]), salt, 600000, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Store) SaveAPIKey(name, value string) error {
	if providerVariables[name] == "" || strings.TrimSpace(value) == "" || len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("供應商或 API Key 無效")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	aead, err := s.keyCipher(salt)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	encrypted := aead.Seal(nil, nonce, []byte(value), []byte(name))
	_, err = s.db.Exec(`INSERT INTO api_keys(name,salt,nonce,ciphertext,created) VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET salt=excluded.salt,nonce=excluded.nonce,ciphertext=excluded.ciphertext,created=excluded.created`, name, salt, nonce, encrypted, time.Now().UTC().Format(time.RFC3339))
	return err
}
func (s *Store) APIKeyRows() ([]APIKeyInfo, error) {
	rows, err := s.db.Query("SELECT id,name,created FROM api_keys ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKeyInfo{}
	for rows.Next() {
		var v APIKeyInfo
		if err = rows.Scan(&v.ID, &v.Name, &v.Created); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) APIKeyValues() ([]APIKeyPair, error) {
	rows, err := s.db.Query("SELECT name,salt,nonce,ciphertext FROM api_keys ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKeyPair{}
	for rows.Next() {
		var name string
		var salt, nonce, ciphertext []byte
		if err = rows.Scan(&name, &salt, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		if len(salt) != 16 || providerVariables[name] == "" {
			return nil, errors.New("API Key 資料損壞")
		}
		aead, err := s.keyCipher(salt)
		if err != nil {
			return nil, err
		}
		if len(nonce) != aead.NonceSize() {
			return nil, errors.New("API Key 資料損壞")
		}
		value, err := aead.Open(nil, nonce, ciphertext, []byte(name))
		if err != nil {
			return nil, errors.New("API Key 解密失敗，請確認管理密碼或重新設定金鑰")
		}
		out = append(out, APIKeyPair{Name: name, Value: string(value)})
		clear(value)
	}
	return out, rows.Err()
}
func (s *Store) DeleteAPIKey(id int64) error {
	_, err := s.db.Exec("DELETE FROM api_keys WHERE id=?", id)
	return err
}
func (s *Server) providerEnv() ([]string, error) {
	values, err := s.store.APIKeyValues()
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, v := range values {
		out = append(out, providerVariables[v.Name]+"="+v.Value)
	}
	return out, nil
}
