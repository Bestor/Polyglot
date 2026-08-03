package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"val-analyzer/internal/dataprovider"
)

func setupTestDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE widgets (sku TEXT, price INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO widgets (sku, price) VALUES ('abc123', 42)"); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestProvider_New_RequiresPath(t *testing.T) {
	p := Provider{}
	if _, err := p.New(context.Background(), map[string]any{}); err == nil {
		t.Error("expected an error when path is missing")
	}
}

func TestProvider_New_BadPath(t *testing.T) {
	p := Provider{}
	if _, err := p.New(context.Background(), map[string]any{"path": "/does/not/exist/data.db"}); err == nil {
		t.Error("expected an error for a nonexistent database file")
	}
}

func TestProvider_New_GoodPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()
}

func TestProvider_New_RejectsOwnDataDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{OwnDataDir: dir}
	if _, err := p.New(context.Background(), map[string]any{"path": dbPath}); err == nil {
		t.Error("expected an error when path resolves inside OwnDataDir")
	}
}

func TestProvider_New_RejectsOwnDataDir_ViaSymlink(t *testing.T) {
	realDir := t.TempDir()
	dbPath := filepath.Join(realDir, "data.db")
	setupTestDB(t, dbPath)

	parent := t.TempDir()
	symlinkedOwnDir := filepath.Join(parent, "own-data-symlink")
	if err := os.Symlink(realDir, symlinkedOwnDir); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	p := Provider{OwnDataDir: symlinkedOwnDir}
	if _, err := p.New(context.Background(), map[string]any{"path": dbPath}); err == nil {
		t.Error("expected an error when path resolves inside OwnDataDir via a symlink")
	}
}

func TestProvider_New_AllowsOutsideOwnDataDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	other := t.TempDir()
	p := Provider{OwnDataDir: other}
	inst, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("expected a path outside OwnDataDir to be allowed, got: %v", err)
	}
	defer inst.Close()
}

func TestInstance_Catalog(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()

	catalog, err := inst.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog) != 1 || catalog[0].Name != "widgets" {
		t.Fatalf("expected one widgets table, got %+v", catalog)
	}
	if len(catalog[0].Columns) != 2 {
		t.Fatalf("expected 2 columns, got %+v", catalog[0].Columns)
	}
}

// TestInstance_Catalog_DetectsForeignKeys proves Catalog() picks up a
// declared SQLite FOREIGN KEY constraint via PRAGMA foreign_key_list and
// sets ReferencesTable/ReferencesColumn accordingly, leaving a plain
// column (including the referenced table's own primary key) untouched.
func TestInstance_Catalog_DetectsForeignKeys(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening setup db: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create authors: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE books (id TEXT PRIMARY KEY, author_id TEXT, FOREIGN KEY(author_id) REFERENCES authors(id))"); err != nil {
		t.Fatalf("create books: %v", err)
	}
	db.Close()

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()

	catalog, err := inst.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}

	var books *dataprovider.TableCatalog
	for i := range catalog {
		if catalog[i].Name == "books" {
			books = &catalog[i]
		}
	}
	if books == nil {
		t.Fatal("books table missing from catalog")
	}

	var authorID, id *dataprovider.ColumnCatalog
	for i := range books.Columns {
		switch books.Columns[i].Name {
		case "author_id":
			authorID = &books.Columns[i]
		case "id":
			id = &books.Columns[i]
		}
	}
	if authorID == nil {
		t.Fatal("author_id column missing from books")
	}
	if authorID.ReferencesTable != "authors" || authorID.ReferencesColumn != "id" {
		t.Errorf("expected relation to authors.id, got references_table=%q references_column=%q",
			authorID.ReferencesTable, authorID.ReferencesColumn)
	}
	if id == nil {
		t.Fatal("id column missing from books")
	}
	if id.ReferencesTable != "" || id.ReferencesColumn != "" {
		t.Errorf("expected no relation on a plain primary key column, got references_table=%q references_column=%q",
			id.ReferencesTable, id.ReferencesColumn)
	}
}

func TestInstance_SampleRows(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{}
	i, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer i.Close()

	sampler, ok := i.(interface {
		SampleRows(ctx context.Context, table string, n int) ([]map[string]any, error)
	})
	if !ok {
		t.Fatal("expected instance to implement RowSampler")
	}
	rows, err := sampler.SampleRows(context.Background(), "widgets", 10)
	if err != nil {
		t.Fatalf("SampleRows: %v", err)
	}
	if len(rows) != 1 || rows[0]["sku"] != "abc123" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestInstance_SampleRows_RejectsInvalidIdentifier(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{}
	i, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer i.Close()

	inst := i.(*instance)
	if _, err := inst.SampleRows(context.Background(), "widgets; DROP TABLE widgets", 10); err == nil {
		t.Error("expected an error for a non-identifier table argument")
	}
}

func TestInstance_Query_DelegatesToRunReadOnlyQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	setupTestDB(t, dbPath)

	p := Provider{}
	i, err := p.New(context.Background(), map[string]any{"path": dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer i.Close()

	result, err := i.Query(context.Background(), "SELECT sku FROM widgets")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "abc123" {
		t.Fatalf("unexpected result: %+v", result)
	}

	if _, err := i.Query(context.Background(), "DELETE FROM widgets"); err == nil {
		t.Error("expected a write statement to be rejected - Query must delegate to ai.RunReadOnlyQuery's safety checks")
	}
}
