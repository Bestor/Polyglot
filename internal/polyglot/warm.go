package polyglot

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
)

type WarmRequest struct {
	Function string         `json:"function"`
	Args     map[string]any `json:"args"`
}

type WarmResponse struct {
	Function string         `json:"function"`
	Summary  string         `json:"summary"`
	Data     map[string]any `json:"data,omitempty"`
}

// handleWarm implements POST /warm: look up the named Function, validate
// its required args are present, then run it. Per-argument type/value
// validation beyond presence is left to each Function's Run.
func handleWarm(functions map[string]Function) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req WarmRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}

		fn, ok := functions[req.Function]
		if !ok {
			return e.BadRequestError(fmt.Sprintf("unknown function %q", req.Function), nil)
		}

		if err := requireArgs(fn.Args, req.Args); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}

		outcome, err := fn.Run(e.Request.Context(), req.Args)
		if err != nil {
			return e.InternalServerError(fmt.Sprintf("%s failed", req.Function), err)
		}

		return e.JSON(http.StatusOK, WarmResponse{
			Function: req.Function,
			Summary:  outcome.Summary,
			Data:     outcome.Data,
		})
	}
}

// requireArgs checks that every required arg (per the function's own
// declared args, the single source of truth also served via GET
// /metadata) is present in the caller-supplied args.
func requireArgs(declared []ai.UpdateArg, provided map[string]any) error {
	for _, arg := range declared {
		if !arg.Required {
			continue
		}
		if _, ok := provided[arg.Name]; !ok {
			return fmt.Errorf("missing required argument %q", arg.Name)
		}
	}
	return nil
}
