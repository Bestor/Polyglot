package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestReadOnlyExecutor_RejectsNonSelect(t *testing.T) {
	dir := t.TempDir()
	query, err := NewReadOnlyExecutor(dir)
	if err != nil {
		t.Fatalf("NewReadOnlyExecutor: %v", err)
	}

	for _, stmt := range []string{
		"DELETE FROM players",
		"INSERT INTO players (id) VALUES ('x')",
		"DROP TABLE players",
	} {
		_, err := query(context.Background(), stmt)
		if !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("expected %q to be rejected with ErrNotReadOnly, got %v", stmt, err)
		}
	}
}

func TestReadOnlyExecutor_Truncates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	if _, err := setup.Exec("CREATE TABLE players (id TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < maxQueryRows+1; i++ {
		if _, err := setup.Exec("INSERT INTO players (id) VALUES (?)", fmt.Sprintf("p%d", i)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("closing setup db: %v", err)
	}

	query, err := NewReadOnlyExecutor(dir)
	if err != nil {
		t.Fatalf("NewReadOnlyExecutor: %v", err)
	}

	result, err := query(context.Background(), "SELECT id FROM players")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != maxQueryRows {
		t.Fatalf("expected %d rows, got %d", maxQueryRows, len(result.Rows))
	}
	if !result.Truncated {
		t.Error("expected Truncated to be true")
	}
}

func TestReadOnlyExecutor_RunsSelect(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	if _, err := setup.Exec("CREATE TABLE players (id TEXT, riot_name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := setup.Exec("INSERT INTO players (id, riot_name) VALUES ('abc', 'Sova')"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("closing setup db: %v", err)
	}

	query, err := NewReadOnlyExecutor(dir)
	if err != nil {
		t.Fatalf("NewReadOnlyExecutor: %v", err)
	}

	result, err := query(context.Background(), "SELECT riot_name FROM players")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if got := result.Rows[0][0]; got != "Sova" {
		t.Fatalf("expected Sova, got %v", got)
	}
	if result.Truncated {
		t.Error("expected Truncated to be false")
	}
}
