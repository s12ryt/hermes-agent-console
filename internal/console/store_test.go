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
