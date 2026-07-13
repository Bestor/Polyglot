#!/bin/sh
# Runs the val-analyzer stack - the polyglot Data API and its MCP server -
# both with DEBUG=true (verbose slog output: full SQL text, request args,
# response bodies; see internal/logging and .env.example). Pass --build
# (or -b) to rebuild the image first; otherwise the existing local image
# is reused.
#
# HENRIK_API_KEY in .env is optional: if set, polyglot auto-onboards the
# valorant datasource on boot; if unset, polyglot still starts with zero
# datasources onboarded - onboard any datasource (including valorant)
# later via POST /datasources. See openapi/polyglot.yaml.
set -e

IMAGE=val-analyzer
NETWORK=val-analyzer-net

BUILD=false
for arg in "$@"; do
  case "$arg" in
    --build|-b) BUILD=true ;;
  esac
done

if [ "$BUILD" = true ]; then
  docker build -t "$IMAGE" .
fi

docker network create "$NETWORK" >/dev/null 2>&1 || true

docker rm -f val-analyzer-polyglot val-analyzer-mcpserver >/dev/null 2>&1 || true

docker run -d --name val-analyzer-polyglot --network "$NETWORK" \
  -p 8091:8091 \
  --env-file .env \
  -e PORT=8091 \
  -e DEBUG=true \
  -v val-analyzer-polyglot-data:/app/pb_data \
  --entrypoint /app/polyglot \
  "$IMAGE"

POLYGLOT_AUTH_TOKEN=$(grep '^API_AUTH_TOKEN=' .env | cut -d= -f2)

docker run -d --name val-analyzer-mcpserver --network "$NETWORK" \
  -p 8092:8092 \
  -e PORT=8092 \
  -e POLYGLOT_URL=http://val-analyzer-polyglot:8091 \
  -e POLYGLOT_AUTH_TOKEN="$POLYGLOT_AUTH_TOKEN" \
  -e DEBUG=true \
  --entrypoint /app/mcpserver \
  "$IMAGE"

echo "polyglot  running at http://localhost:8091          (docker logs -f val-analyzer-polyglot)"
echo "mcpserver running at http://localhost:8092/mcp      (docker logs -f val-analyzer-mcpserver)"
echo "both started with DEBUG=true - full SQL/args/response bodies will show up in their logs"
