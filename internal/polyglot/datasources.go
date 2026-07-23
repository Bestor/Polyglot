package polyglot

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type ConfigFieldDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
}

type ProviderTypeDescription struct {
	Type   string                   `json:"type"`
	Config []ConfigFieldDescription `json:"config"`
}

type ActiveDatasourceDescription struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type DatasourcesResponse struct {
	AvailableTypes []ProviderTypeDescription     `json:"available_types"`
	Active         []ActiveDatasourceDescription `json:"active"`
}

type OnboardRequest struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type OnboardResponse struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Config is already secret-safe: any Secret-flagged field is a
	// SecretRef ({"$vault": "..."}), never the literal value - see
	// PersistConfig.
	Config         map[string]any `json:"config"`
	ReconcileJobID string         `json:"reconcile_job_id"`
}

// handleListDatasources implements GET /datasources: every compiled-in
// provider type (with its config schema) plus every currently active
// datasource instance.
func handleListDatasources(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error { return e.JSON(http.StatusOK, reg.List()) }
}

// handleOnboardDatasource implements POST /datasources: onboard (or
// idempotently reconfigure) a datasource instance of a compiled-in
// provider type, under a caller-chosen name.
func handleOnboardDatasource(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req OnboardRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.Name == "" {
			return e.BadRequestError("name is required", nil)
		}
		if req.Type == "" {
			return e.BadRequestError("type is required", nil)
		}

		result, err := reg.Onboard(e.Request.Context(), e.App, req.Name, req.Type, req.Config)
		switch {
		case err == nil:
			return e.JSON(http.StatusOK, result)
		case errors.Is(err, errUnknownProviderType), errors.Is(err, errInvalidConfig), errors.Is(err, errReservedName):
			return e.BadRequestError(err.Error(), nil)
		default:
			return e.InternalServerError("onboarding failed", err)
		}
	}
}

type ReconcileRequest struct {
	Name string `json:"name"`
}

// handleReconcileDatasource implements POST /datasources/reconcile:
// re-run catalog reconciliation for an already-onboarded datasource,
// asynchronously - 202 + a job id, pollable via GET /jobs?id=.
func handleReconcileDatasource(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req ReconcileRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.Name == "" {
			return e.BadRequestError("name is required", nil)
		}

		job, err := reg.Reconcile(e.App, req.Name)
		if err != nil {
			if errors.Is(err, errUnknownDatasource) {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.InternalServerError("reconcile failed", err)
		}
		return e.JSON(http.StatusAccepted, job)
	}
}

type AnnotateDatasourceRequest struct {
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	QueryGuidance *string `json:"query_guidance"`
}

// handleAnnotateDatasource implements POST /datasources/annotate: patches
// a datasource's connection-level description/query_guidance. Pointer
// fields so "omitted" and "explicitly cleared to empty" are
// distinguishable - a real partial update.
func handleAnnotateDatasource() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AnnotateDatasourceRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.Name == "" {
			return e.BadRequestError("name is required", nil)
		}

		rec, err := e.App.FindFirstRecordByFilter(datasourcesCollection, "name = {:name}", dbx.Params{"name": req.Name})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return e.NotFoundError(fmt.Sprintf("unknown datasource %q", req.Name), nil)
			}
			return e.InternalServerError("lookup failed", err)
		}

		if req.Description != nil {
			rec.Set("description", *req.Description)
		}
		if req.QueryGuidance != nil {
			rec.Set("query_guidance", *req.QueryGuidance)
		}
		if err := e.App.Save(rec); err != nil {
			return e.InternalServerError("save failed", err)
		}
		return e.JSON(http.StatusOK, map[string]string{"name": req.Name})
	}
}

type AnnotateTableRequest struct {
	ID            string  `json:"id"`
	Description   *string `json:"description"`
	QueryGuidance *string `json:"query_guidance"`
}

// handleAnnotateTable implements POST /tables/annotate: patches one
// table's curated description/query_guidance, looked up by the id
// GET /metadata exposes.
func handleAnnotateTable() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AnnotateTableRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.ID == "" {
			return e.BadRequestError("id is required", nil)
		}

		rec, err := e.App.FindRecordById("tables", req.ID)
		if err != nil {
			return e.NotFoundError(fmt.Sprintf("unknown table %q", req.ID), nil)
		}

		if req.Description != nil {
			rec.Set("description", *req.Description)
		}
		if req.QueryGuidance != nil {
			rec.Set("query_guidance", *req.QueryGuidance)
		}
		if err := e.App.Save(rec); err != nil {
			return e.InternalServerError("save failed", err)
		}
		return e.JSON(http.StatusOK, map[string]string{"id": req.ID})
	}
}

type AnnotateColumnRequest struct {
	ID          string  `json:"id"`
	Description *string `json:"description"`
}

// handleAnnotateColumn implements POST /columns/annotate: patches one
// column's curated description - no query_guidance field, unlike tables
// (table-level prose can name specific columns; a third tier is
// over-engineering).
func handleAnnotateColumn() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AnnotateColumnRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.ID == "" {
			return e.BadRequestError("id is required", nil)
		}

		rec, err := e.App.FindRecordById("columns", req.ID)
		if err != nil {
			return e.NotFoundError(fmt.Sprintf("unknown column %q", req.ID), nil)
		}

		if req.Description != nil {
			rec.Set("description", *req.Description)
		}
		if err := e.App.Save(rec); err != nil {
			return e.InternalServerError("save failed", err)
		}
		return e.JSON(http.StatusOK, map[string]string{"id": req.ID})
	}
}
