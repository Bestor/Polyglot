package dataprovider

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldNumber   FieldType = "number"
	FieldBool     FieldType = "bool"
	FieldDate     FieldType = "date"
	FieldRelation FieldType = "relation"
	FieldJSON     FieldType = "json"
	FieldAutodate FieldType = "autodate"
)

// FieldSpec describes one column: enough to build a real core.Field for
// dynamic collection creation (ToCoreField) and to describe a column for
// GET /metadata (Description).
type FieldSpec struct {
	Name        string
	Type        FieldType
	Description string
	Required    bool

	Max           int    // FieldText max length (0 = PocketBase default)
	OnlyInt       bool   // FieldNumber: restrict to integers
	RelationTable string // FieldRelation: target collection name
	CascadeDelete bool   // FieldRelation
	MaxSize       int64  // FieldJSON: max byte size (0 = PocketBase default)
	OnCreate      bool   // FieldAutodate
	OnUpdate      bool   // FieldAutodate
}

// ToCoreField converts f into the concrete core.Field PocketBase needs.
// For FieldRelation it resolves RelationTable to that collection's id via
// app, which must already exist - see the ordering requirement on
// Provider.Tables().
func (f FieldSpec) ToCoreField(app core.App) (core.Field, error) {
	switch f.Type {
	case FieldText:
		return &core.TextField{Name: f.Name, Required: f.Required, Max: f.Max}, nil
	case FieldNumber:
		return &core.NumberField{Name: f.Name, Required: f.Required, OnlyInt: f.OnlyInt}, nil
	case FieldBool:
		return &core.BoolField{Name: f.Name, Required: f.Required}, nil
	case FieldDate:
		return &core.DateField{Name: f.Name, Required: f.Required}, nil
	case FieldJSON:
		return &core.JSONField{Name: f.Name, Required: f.Required, MaxSize: f.MaxSize}, nil
	case FieldAutodate:
		return &core.AutodateField{Name: f.Name, OnCreate: f.OnCreate, OnUpdate: f.OnUpdate}, nil
	case FieldRelation:
		target, err := app.FindCollectionByNameOrId(f.RelationTable)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("field %q: relation target table %q does not exist yet (Tables() must list it earlier)", f.Name, f.RelationTable)
			}
			return nil, err
		}
		return &core.RelationField{Name: f.Name, Required: f.Required, CollectionId: target.Id, CascadeDelete: f.CascadeDelete}, nil
	default:
		return nil, fmt.Errorf("field %q: unknown field type %q", f.Name, f.Type)
	}
}

type IndexSpec struct {
	Name    string
	Unique  bool
	Columns []string
}

// TableSpec describes one PocketBase collection a provider owns.
type TableSpec struct {
	Name        string
	Description string
	Fields      []FieldSpec
	Indexes     []IndexSpec
}
