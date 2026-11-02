package console

import "testing"

func TestConversationFullTextSearch(t *testing.T) {
	s := testServer(t).store
	if err := s.indexConversation("s1", "使用者：資料庫備份\n助理：SQLite backup guide"); err != nil {
		t.Fatal(err)
	}
	if err := s.indexConversation("s2", "another unrelated topic"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"SQLite", "資料庫", "\" OR *"} {
		hits, err := s.searchConversations(query)
		if err != nil {
			t.Fatal(err)
		}
		if query != "\" OR *" && (len(hits) != 1 || hits[0].SessionID != "s1") {
			t.Fatalf("%s: %+v", query, hits)
		}
	}
	if err := s.indexConversation("s1", "replacement content"); err != nil {
		t.Fatal(err)
	}
	hits, err := s.searchConversations("SQLite")
	if err != nil || len(hits) != 0 {
		t.Fatalf("stale index %+v %v", hits, err)
	}
}
