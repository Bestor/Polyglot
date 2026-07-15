package cachewarmer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadPlayerTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.txt")
	content := "# a comment\n\nOrBest#NA1\n  \n# another comment\ngoatninja01#NA1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadPlayerTags(path)
	if err != nil {
		t.Fatalf("ReadPlayerTags: %v", err)
	}

	want := []string{"OrBest#NA1", "goatninja01#NA1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestReadPlayerTags_MissingFile(t *testing.T) {
	got, err := ReadPlayerTags(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("expected no error for a missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a missing file, got %v", got)
	}
}
