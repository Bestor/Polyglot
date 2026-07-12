// Package mcpserver generates an MCP tool for every operation in
// openapi/polyglot.yaml and proxies each tool call to the polyglot Data
// API over HTTP. Tool schemas are derived from the spec at load time, so
// they can never drift from the actual REST contract - there is no
// hand-maintained, hardcoded copy of polyglot's API surface here.
package mcpserver

import (
	"fmt"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Param describes one query parameter an Operation's tool input maps onto
// a request URL with. Operations in this spec never mix a request body
// with query parameters, so Params is only populated for body-less
// operations (see Operation.HasBody).
type Param struct {
	Name string
}

// Operation is everything needed to expose one OpenAPI operation as an
// MCP tool and, on a tool call, turn its arguments back into an HTTP
// request against polyglot.
type Operation struct {
	Name        string // from operationId
	Description string
	Method      string
	Path        string
	InputSchema map[string]any

	// Params holds the query parameters expected when HasBody is false.
	Params []Param
	// HasBody is true when the operation takes a JSON request body - in
	// that case, a tool call's entire arguments map is marshaled as-is to
	// become that body (the request body's own schema is what became
	// InputSchema, so the shapes match by construction).
	HasBody bool
}

// LoadOperations parses the OpenAPI document at specPath and returns one
// Operation per path+method defined in it.
func LoadOperations(specPath string) ([]Operation, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("reading openapi spec: %w", err)
	}

	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing openapi spec: %w", err)
	}

	docModel, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("building openapi v3 model: %w", err)
	}

	if docModel.Model.Paths == nil || docModel.Model.Paths.PathItems == nil {
		return nil, fmt.Errorf("openapi spec at %s defines no paths", specPath)
	}

	var ops []Operation
	for path, item := range docModel.Model.Paths.PathItems.FromOldest() {
		for _, mo := range []struct {
			method string
			op     *v3.Operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"PATCH", item.Patch},
			{"DELETE", item.Delete},
		} {
			if mo.op == nil {
				continue
			}
			op, err := buildOperation(mo.method, path, mo.op)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", mo.method, path, err)
			}
			ops = append(ops, op)
		}
	}

	return ops, nil
}

func buildOperation(method, path string, op *v3.Operation) (Operation, error) {
	name := op.OperationId
	if name == "" {
		return Operation{}, fmt.Errorf("missing operationId")
	}

	description := op.Description
	if description == "" {
		description = op.Summary
	}

	result := Operation{
		Name:        name,
		Description: description,
		Method:      method,
		Path:        path,
	}

	if op.RequestBody != nil {
		result.HasBody = true
		result.InputSchema = requestBodySchema(op.RequestBody)
		return result, nil
	}

	properties := map[string]any{}
	var required []string
	for _, p := range op.Parameters {
		if !strings.EqualFold(p.In, "query") {
			continue
		}

		propSchema := map[string]any{}
		if p.Schema != nil {
			propSchema = schemaProxyToMap(p.Schema)
		}
		if p.Description != "" {
			propSchema["description"] = p.Description
		}

		properties[p.Name] = propSchema
		result.Params = append(result.Params, Param{Name: p.Name})
		if p.Required != nil && *p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]any{"type": "object"}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	result.InputSchema = schema

	return result, nil
}

// requestBodySchema returns the JSON schema for an operation's
// application/json request body, or a bare {"type":"object"} if none is
// declared with that content type.
func requestBodySchema(rb *v3.RequestBody) map[string]any {
	if rb.Content != nil {
		if mt := rb.Content.GetOrZero("application/json"); mt != nil && mt.Schema != nil {
			return schemaProxyToMap(mt.Schema)
		}
	}
	return map[string]any{"type": "object"}
}

// schemaProxyToMap resolves sp and converts it to a JSON-schema-shaped
// map[string]any (the form MCP's Tool.InputSchema accepts). It covers the
// subset of JSON Schema that openapi/polyglot.yaml actually uses -
// object/string/integer/number/boolean/array types, properties, required,
// items, and additionalProperties - not the full JSON Schema 2020-12
// vocabulary base.Schema can represent.
func schemaProxyToMap(sp *base.SchemaProxy) map[string]any {
	if sp == nil {
		return map[string]any{}
	}
	return schemaToMap(sp.Schema())
}

func schemaToMap(s *base.Schema) map[string]any {
	if s == nil {
		return map[string]any{}
	}

	out := map[string]any{}

	switch len(s.Type) {
	case 0:
		// no explicit type - leave it unset, matching the source schema.
	case 1:
		out["type"] = s.Type[0]
	default:
		out["type"] = s.Type
	}

	if s.Description != "" {
		out["description"] = s.Description
	}

	if s.Properties != nil {
		properties := map[string]any{}
		for name, propProxy := range s.Properties.FromOldest() {
			properties[name] = schemaProxyToMap(propProxy)
		}
		out["properties"] = properties
	}

	if len(s.Required) > 0 {
		out["required"] = s.Required
	}

	if s.Items != nil {
		switch {
		case s.Items.IsA():
			out["items"] = schemaProxyToMap(s.Items.A)
		case s.Items.IsB():
			out["items"] = s.Items.B
		}
	}

	if s.AdditionalProperties != nil {
		switch {
		case s.AdditionalProperties.IsA():
			out["additionalProperties"] = schemaProxyToMap(s.AdditionalProperties.A)
		case s.AdditionalProperties.IsB():
			out["additionalProperties"] = s.AdditionalProperties.B
		}
	}

	return out
}
