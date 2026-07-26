#!/bin/sh
# Quick manual test of POST /warm against a locally running polyglot
# container (see run.sh) - starts a sync_matches job for a player's
# recent match history, without going through the MCP server or an AI at
# all. polyglot's own POST /warm is a thin proxy to whichever onboarded
# datasource implements dataprovider.FunctionRunner (currently just
# valorant, via internal/providers/httpsql) - the request/job shape is
# valorantapi's own, just with an added "datasource" field. /warm is
# asynchronous: this prints a 202 + job id immediately, not the finished
# result - see the GET /jobs example below to poll it (not GET /warm?id= -
# polyglot itself has no GET /warm route; polling is unified under
# GET /jobs?id=[&datasource=] the same way /query's own ?datasource=
# routes to a named datasource).
set -e

TOKEN=$(grep '^API_AUTH_TOKEN=' .env | cut -d= -f2)

curl -s -X POST http://localhost:8091/warm \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "datasource": "valorant",
    "function": "sync_matches",
    "args": {
      "player_tag": "OrBest#NA1",
      "count": 100
    }
  }'

echo
echo "poll job status with:"
echo "  curl -s -H \"Authorization: Bearer \$TOKEN\" 'http://localhost:8091/jobs?id=<id from above>&datasource=valorant'"
