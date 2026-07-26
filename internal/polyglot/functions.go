package polyglot

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
)

type WarmRequest struct {
	Datasource string         `json:"datasource"`
	Function   string         `json:"function"`
	Args       map[string]any `json:"args"`
}

// handleWarm implements POST /warm: proxies a named-function trigger
// through to datasource's own dataprovider.FunctionRunner capability -
// currently only cmd/valorantapi's own /warm, reached via
// internal/providers/httpsql - and returns 202 + the remote's own Job,
// pollable via GET /jobs?id=&datasource=.
func handleWarm(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req WarmRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.Datasource == "" {
			return e.BadRequestError("datasource is required", nil)
		}
		if req.Function == "" {
			return e.BadRequestError("function is required", nil)
		}

		job, err := reg.RunFunction(e.Request.Context(), req.Datasource, req.Function, req.Args)
		if err != nil {
			return functionErrorResponse(e, err)
		}
		return e.JSON(http.StatusAccepted, job)
	}
}

// functionErrorResponse classifies an error from Registry.RunFunction/
// FunctionJobStatus into the right HTTP status: polyglot's own "unknown
// datasource"/"doesn't support functions" are always caller error (400); a
// dataprovider.RemoteError passes through whatever exact status the remote
// datasource itself already decided (valorantapi's own /warm already
// returns proper 400s for an unknown function/missing arg, and 404 for an
// unknown job id); anything else (a network failure, an unreachable
// remote) is a genuine 500.
func functionErrorResponse(e *core.RequestEvent, err error) error {
	if errors.Is(err, errUnknownDatasource) || errors.Is(err, errFunctionsNotSupported) {
		return e.BadRequestError(err.Error(), nil)
	}
	var remoteErr *dataprovider.RemoteError
	if errors.As(err, &remoteErr) {
		return e.Error(remoteErr.StatusCode, remoteErr.Message, nil)
	}
	return e.InternalServerError("function call failed", err)
}

type AnnotateFunctionRequest struct {
	ID            string  `json:"id"`
	QueryGuidance *string `json:"query_guidance"`
}

// handleAnnotateFunction implements POST /functions/annotate: patches one
// function's curated query_guidance only - no description field, mirroring
// handleAnnotateColumn's narrower field set (unlike handleAnnotateTable/
// handleAnnotateDatasource): a function's description already comes
// meaningfully from the provider's own code and is refreshed every
// reconcile, so there's nothing to hand-curate there.
func handleAnnotateFunction() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AnnotateFunctionRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.ID == "" {
			return e.BadRequestError("id is required", nil)
		}

		rec, err := e.App.FindRecordById("functions", req.ID)
		if err != nil {
			return e.NotFoundError(fmt.Sprintf("unknown function %q", req.ID), nil)
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
