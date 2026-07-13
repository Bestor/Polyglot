#!/bin/sh
# Quick manual test of POST /warm against a locally running polyglot
# container (see run.sh) - pre-loads a player's recent match history into
# the cache via the sync_matches function, without going through the MCP
# server or an AI at all.
set -e

curl -s -X POST http://localhost:8091/warm \
  -H "Authorization: Bearer $(grep '^API_AUTH_TOKEN=' .env | cut -d= -f2)" \
  -H "Content-Type: application/json" \
  -d '{
    "function": "sync_matches",
    "args": {
      "player_tag": "OrBest#NA1",
      "count": 100
    }
  }'
