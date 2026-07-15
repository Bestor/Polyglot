#!/bin/sh
# Quick manual test of POST /warm against a locally running polyglot
# container (see run.sh) - starts a sync_matches job for a player's
# recent match history, without going through the MCP server or an AI at
# all. /warm is asynchronous: this prints a 202 + job id immediately, not
# the finished result - see the GET /warm?id= example below to poll it.
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
echo "  curl -s -H \"Authorization: Bearer \$TOKEN\" 'http://localhost:8091/warm?id=<id from above>'"
