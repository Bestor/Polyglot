// Package sqlite is a dataprovider.Provider that connects to an existing
// local SQLite file, read-only. It never creates or migrates schema - the
// file must already exist and be structured however its owner built it;
// this provider only ever opens it in mode=ro and introspects/queries it.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
)

// Type is this provider's stable slug, used as the registry key.
const Type = "sqlite"

// Provider bakes in polyglot's own data dir so New can refuse to connect
// to polyglot's own bookkeeping storage - see rejectOwnDataDir.
type Provider struct{ OwnDataDir string }

var _ dataprovider.Provider = Provider{}

func (Provider) Type() string { return Type }

func (Provider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{
		{Name: "path", Type: "string", Required: true,
			Description: "Absolute filesystem path to the SQLite database file, readable by the polyglot process."},
	}
}

func (p Provider) New(ctx context.Context, config map[string]any) (dataprovider.Instance, error) {
	path, _ := config["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("sqlite: path is required")
	}
	if err := rejectOwnDataDir(path, p.OwnDataDir); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening %q: %w", path, err)
	}
	// sql.Open never dials - force a real round trip so a bad path fails
	// onboarding, not the first query.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: connecting to %q: %w", path, err)
	}

	return &instance{db: db}, nil
}

// rejectOwnDataDir is the MUST-FIX security guard: without it, onboarding
// a SQLite datasource whose path resolves inside polyglot's own data
// directory would let GET /query?datasource=<name>&sql=SELECT config FROM
// datasources read back every other datasource's vault path references -
// not raw secrets post-vault-migration, but still a real information-
// disclosure surface worth closing outright, not just relying on
// reservedTablePattern as the only layer. Robust to relative paths and
// symlinks via filepath.Abs + filepath.EvalSymlinks containment checks.
func rejectOwnDataDir(path, ownDataDir string) error {
	if ownDataDir == "" {
		return nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sqlite: resolving path %q: %w", path, err)
	}
	absOwnDir, err := filepath.Abs(ownDataDir)
	if err != nil {
		return fmt.Errorf("sqlite: resolving own data dir %q: %w", ownDataDir, err)
	}

	// EvalSymlinks can fail if the target doesn't exist yet (e.g. own data
	// dir not yet created) - fall back to the unresolved absolute path in
	// that case rather than failing onboarding outright.
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absOwnDir); err == nil {
		absOwnDir = resolved
	}

	rel, err := filepath.Rel(absOwnDir, absPath)
	if err != nil {
		return nil // different volumes/roots - can't be contained
	}
	if rel == "." || (!strings.HasPrefix(rel, "..") && rel != "") {
		return fmt.Errorf("sqlite: path %q resolves inside polyglot's own data directory, which is not allowed", path)
	}
	return nil
}

type instance struct{ db *sql.DB }

var _ dataprovider.Instance = (*instance)(nil)
var _ dataprovider.RowSampler = (*instance)(nil)

// identRe matches a plain SQLite identifier - the only shape Catalog ever
// feeds back into quoteIdent (straight from sqlite_master), and the only
// shape SampleRows accepts from a caller (rejecting anything else instead
// of trying to safely quote arbitrary caller input).
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteIdent(name string) (string, error) {
	if !identRe.MatchString(name) {
		return "", fmt.Errorf("sqlite: invalid identifier %q", name)
	}
	return `"` + name + `"`, nil
}

func (i *instance) Catalog(ctx context.Context) ([]dataprovider.TableCatalog, error) {
	rows, err := i.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("sqlite: listing tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sqlite: scanning table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	tables := make([]dataprovider.TableCatalog, 0, len(names))
	for _, name := range names {
		quoted, err := quoteIdent(name)
		if err != nil {
			return nil, err // sqlite_master returned something identRe rejects - shouldn't happen, fail loudly rather than silently skip
		}
		colRows, err := i.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoted))
		if err != nil {
			return nil, fmt.Errorf("sqlite: introspecting table %q: %w", name, err)
		}
		var columns []dataprovider.ColumnCatalog
		for colRows.Next() {
			var cid int
			var colName, colType string
			var notNull, pk int
			var dfltValue any
			if err := colRows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err != nil {
				colRows.Close()
				return nil, fmt.Errorf("sqlite: scanning column info for %q: %w", name, err)
			}
			columns = append(columns, dataprovider.ColumnCatalog{Name: colName, Type: colType})
		}
		if err := colRows.Err(); err != nil {
			colRows.Close()
			return nil, err
		}
		colRows.Close()

		tables = append(tables, dataprovider.TableCatalog{Name: name, Columns: columns})
	}

	return tables, nil
}

func (i *instance) SampleRows(ctx context.Context, table string, n int) ([]map[string]any, error) {
	quoted, err := quoteIdent(table)
	if err != nil {
		return nil, err
	}

	rows, err := i.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT ?", quoted), n)
	if err != nil {
		return nil, fmt.Errorf("sqlite: sampling %q: %w", table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (i *instance) Query(ctx context.Context, sqlText string) (ai.QueryResult, error) {
	return ai.RunReadOnlyQuery(ctx, i.db, sqlText)
}

func (i *instance) Close() error { return i.db.Close() }
