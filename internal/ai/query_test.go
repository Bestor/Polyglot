package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

// TestReadOnlyExecutor_TruncatesOversizedCell reproduces the actual
// incident that motivated maxCellBytes: a query touching a wide column
// (e.g. matches.raw_json, up to 4MiB/row) returned a single cell large
// enough on its own to blow an LLM caller's context window, even though it
// was only one row - well under maxQueryRows. The row-count cap alone
// never would have caught this.
func TestReadOnlyExecutor_TruncatesOversizedCell(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	if _, err := setup.Exec("CREATE TABLE matches (id TEXT, raw_json TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	huge := strings.Repeat("x", maxCellBytes*4)
	if _, err := setup.Exec("INSERT INTO matches (id, raw_json) VALUES ('m1', ?)", huge); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("closing setup db: %v", err)
	}

	query, err := NewReadOnlyExecutor(dir)
	if err != nil {
		t.Fatalf("NewReadOnlyExecutor: %v", err)
	}

	result, err := query(context.Background(), "SELECT raw_json FROM matches")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row (the oversized cell should be truncated, not dropped), got %d", len(result.Rows))
	}
	got, ok := result.Rows[0][0].(string)
	if !ok {
		t.Fatalf("expected a string value, got %T", result.Rows[0][0])
	}
	if len(got) > maxCellBytes+64 { // small allowance for the "...(truncated, N bytes total)" marker
		t.Errorf("expected the cell to be truncated to ~%d bytes, got %d bytes", maxCellBytes, len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected a truncation marker in the returned value, got %q", got[:100])
	}
	if !result.Truncated {
		t.Error("expected Truncated to be true")
	}
}

// TestReadOnlyExecutor_TruncatesCumulativeResponseBytes covers the other
// half of the fix: many rows, each individually under maxCellBytes, that
// still add up to more than maxQueryResponseBytes in total.
func TestReadOnlyExecutor_TruncatesCumulativeResponseBytes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	if _, err := setup.Exec("CREATE TABLE matches (id TEXT, raw_json TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Each row is well under maxCellBytes on its own, but there are enough
	// of them to exceed maxQueryResponseBytes cumulatively, long before
	// maxQueryRows would ever kick in.
	chunk := strings.Repeat("y", maxCellBytes/2)
	rowsNeeded := maxQueryResponseBytes/(maxCellBytes/2) + 5
	for i := 0; i < rowsNeeded; i++ {
		if _, err := setup.Exec("INSERT INTO matches (id, raw_json) VALUES (?, ?)", fmt.Sprintf("m%d", i), chunk); err != nil {
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

	result, err := query(context.Background(), "SELECT raw_json FROM matches")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) >= rowsNeeded {
		t.Fatalf("expected fewer than the %d inserted rows back (cumulative byte cap should stop early), got %d", rowsNeeded, len(result.Rows))
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected at least one row back, got none")
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

// TestRunReadOnlyQuery_DirectCall proves RunReadOnlyQuery works against any
// caller-supplied *sql.DB, not just one built by NewReadOnlyExecutor - the
// property internal/providers/sqlite relies on to reuse this exact
// truncation/safety logic against an onboarded external file.
func TestRunReadOnlyQuery_DirectCall(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE widgets (sku TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO widgets (sku) VALUES ('abc123')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := RunReadOnlyQuery(context.Background(), db, "SELECT sku FROM widgets")
	if err != nil {
		t.Fatalf("RunReadOnlyQuery: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "abc123" {
		t.Fatalf("expected [[abc123]], got %v", result.Rows)
	}

	if _, err := RunReadOnlyQuery(context.Background(), db, "DELETE FROM widgets"); !errors.Is(err, ErrNotReadOnly) {
		t.Errorf("expected DELETE to be rejected with ErrNotReadOnly, got %v", err)
	}
}
