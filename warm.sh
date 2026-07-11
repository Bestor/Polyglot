#!/bin/sh
# Quick manual test of POST /api/warm against a locally running val-analyzer
# container (see run.sh) - pre-loads a player's recent match history into
# the cache without going through the AI.
set -e

curl -s -X POST http://localhost:8090/api/warm \
  -H "Authorization: Bearer $(grep '^API_AUTH_TOKEN=' .env | cut -d= -f2)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "OrBest",
    "tag": "NA1",
    "all": true
  }'
