package warmer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchlist.txt")
	content := "# a comment\n\nOrBest#NA1\n  \n# another comment\ngoatninja01#NA1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadLines(path)
	if err != nil {
		t.Fatalf("ReadLines: %v", err)
	}

	want := []string{"OrBest#NA1", "goatninja01#NA1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestReadLines_MissingFile(t *testing.T) {
	got, err := ReadLines(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("expected no error for a missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a missing file, got %v", got)
	}
}
