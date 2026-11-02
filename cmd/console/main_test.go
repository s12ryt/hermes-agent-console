package main

import (
	"agentconsole/web"
	"io/fs"
	"testing"
)

func TestStartupValidation(t *testing.T) {
	if err := validate("a-long-test-password", "127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	for _, v := range [][2]string{{"", "127.0.0.1:8080"}, {"short", "127.0.0.1:8080"}, {"a-long-test-password", "invalid"}} {
		if validate(v[0], v[1]) == nil {
			t.Fatalf("accepted invalid configuration %v", v[1])
		}
	}
}
func TestEmbeddedFrontend(t *testing.T) {
	assets := web.Assets()
	raw, err := fs.ReadFile(assets, "index.html")
	if err != nil || len(raw) == 0 {
		t.Fatal("missing index", err)
	}
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil || len(entries) < 2 {
		t.Fatal("missing compiled JS/CSS", err)
	}
}
