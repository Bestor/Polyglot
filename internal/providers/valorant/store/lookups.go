package store

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// MapStore, AgentStore, WeaponStore, and TierStore are small dimension
// tables opportunistically populated from observed match data (like
// PlayerStore), rather than seeded upfront - self-maintaining, and never
// blocks ingestion on a hardcoded/stale list of known maps, agents,
// weapons, or tiers.

type MapRow struct {
	ID    string
	MapID string
	Name  string
}

type MapStore struct{ app core.App }

func NewMapStore(app core.App) *MapStore { return &MapStore{app: app} }

func (s *MapStore) Upsert(mapID, name string) (MapRow, error) {
	col, err := s.app.FindCollectionByNameOrId("maps")
	if err != nil {
		return MapRow{}, err
	}

	rec, err := s.app.FindFirstRecordByFilter("maps", "map_id = {:id}", dbx.Params{"id": mapID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MapRow{}, err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("map_id", mapID)
	}
	if name != "" {
		rec.Set("name", name)
	}
	if err := s.app.Save(rec); err != nil {
		return MapRow{}, err
	}

	return MapRow{ID: rec.Id, MapID: rec.GetString("map_id"), Name: rec.GetString("name")}, nil
}

type AgentRow struct {
	ID      string
	AgentID string
	Name    string
}

type AgentStore struct{ app core.App }

func NewAgentStore(app core.App) *AgentStore { return &AgentStore{app: app} }

func (s *AgentStore) Upsert(agentID, name string) (AgentRow, error) {
	col, err := s.app.FindCollectionByNameOrId("agents")
	if err != nil {
		return AgentRow{}, err
	}

	rec, err := s.app.FindFirstRecordByFilter("agents", "agent_id = {:id}", dbx.Params{"id": agentID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AgentRow{}, err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("agent_id", agentID)
	}
	if name != "" {
		rec.Set("name", name)
	}
	if err := s.app.Save(rec); err != nil {
		return AgentRow{}, err
	}

	return AgentRow{ID: rec.Id, AgentID: rec.GetString("agent_id"), Name: rec.GetString("name")}, nil
}

type WeaponRow struct {
	ID       string
	WeaponID string
	Name     string
	Type     string
}

type WeaponStore struct{ app core.App }

func NewWeaponStore(app core.App) *WeaponStore { return &WeaponStore{app: app} }

// Upsert stores a weapon/ability/bomb by its natural weapon_id, which may
// be an empty string (the bomb). A completely empty weapon (no id, no
// name) carries no useful identity, so it's skipped rather than upserted
// as a bare placeholder row.
func (s *WeaponStore) Upsert(weaponID, name, weaponType string) (WeaponRow, bool, error) {
	if weaponID == "" && name == "" {
		return WeaponRow{}, false, nil
	}

	col, err := s.app.FindCollectionByNameOrId("weapons")
	if err != nil {
		return WeaponRow{}, false, err
	}

	rec, err := s.app.FindFirstRecordByFilter("weapons", "weapon_id = {:id}", dbx.Params{"id": weaponID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return WeaponRow{}, false, err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("weapon_id", weaponID)
	}
	if name != "" {
		rec.Set("name", name)
	}
	if weaponType != "" {
		rec.Set("type", weaponType)
	}
	if err := s.app.Save(rec); err != nil {
		return WeaponRow{}, false, err
	}

	return WeaponRow{ID: rec.Id, WeaponID: rec.GetString("weapon_id"), Name: rec.GetString("name"), Type: rec.GetString("type")}, true, nil
}

type TierRow struct {
	ID     string
	TierID int
	Name   string
}

type TierStore struct{ app core.App }

func NewTierStore(app core.App) *TierStore { return &TierStore{app: app} }

func (s *TierStore) Upsert(tierID int, name string) (TierRow, error) {
	col, err := s.app.FindCollectionByNameOrId("tiers")
	if err != nil {
		return TierRow{}, err
	}

	rec, err := s.app.FindFirstRecordByFilter("tiers", "tier_id = {:id}", dbx.Params{"id": tierID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TierRow{}, err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("tier_id", tierID)
	}
	if name != "" {
		rec.Set("name", name)
	}
	if err := s.app.Save(rec); err != nil {
		return TierRow{}, err
	}

	return TierRow{ID: rec.Id, TierID: rec.GetInt("tier_id"), Name: rec.GetString("name")}, nil
}
