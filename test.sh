#!/bin/sh
# Quick manual test of the MCP server's "query" tool (POST /mcp) against a
# locally running mcpserver container (see run.sh). mcpserver runs
# stateless (see StreamableHTTPOptions.Stateless in cmd/mcpserver/main.go),
# so a single tools/call request works without a prior initialize
# handshake. mcpserver attaches its own POLYGLOT_AUTH_TOKEN when proxying
# to polyglot, so no Authorization header is needed here.
set -e

curl -s -X POST http://localhost:8092/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "query",
      "arguments": {
        "sql": "SELECT riot_name, riot_tag, region FROM players LIMIT 5"
      }
    }
  }'
