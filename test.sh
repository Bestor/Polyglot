#!/bin/sh
# Quick manual test of POST /api/ask against a locally running val-analyzer
# container (see run.sh).
set -e

curl -s -X POST http://localhost:8090/api/ask \
  -H "Authorization: Bearer $(grep '^API_AUTH_TOKEN=' .env | cut -d= -f2)" \
  -H "Content-Type: application/json" \
  -d '{
    "question": "What was goatninja01#NA1s best match by kills?"
  }'
