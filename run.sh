#!/bin/sh
# Runs the val-analyzer stack via docker compose - the polyglot Data API,
# its MCP server, and (if enabled) a Discord bot. All app configuration
# (ports, volumes, inter-service URLs, the optional discordbot profile)
# lives in docker-compose.yml; secrets/values come from .env (see
# .env.example). This script just drives `docker compose up`.
#
# Pass --build (or -b) to rebuild images first; otherwise existing images
# are reused.
#
# The discordbot service only starts if the "discordbot" Compose profile is
# active - set COMPOSE_PROFILES=discordbot in .env once DISCORD_BOT_TOKEN
# (and ANTHROPIC_API_KEY) are set, since cmd/discordbot fails fast without
# them.
set -e

BUILD=false
for arg in "$@"; do
  case "$arg" in
    --build|-b) BUILD=true ;;
  esac
done

if [ "$BUILD" = true ]; then
  docker compose up -d --build
else
  docker compose up -d
fi

docker compose ps
