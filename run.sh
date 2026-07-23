#!/bin/sh
# Runs the val-analyzer stack via docker compose - the polyglot Data API,
# its MCP server, and (if enabled) a Discord bot and cache warmer. All app
# configuration (ports, volumes, inter-service URLs, the optional
# discordbot profile) lives in docker-compose.yml; secrets/values come
# from .env (see .env.example). This script just drives `docker compose up`.
#
# Pass --build (or -b) to rebuild images first; otherwise existing images
# are reused.
#
# Pass --dev to start ONLY polyglot + mcpserver - no discordbot, no
# cachewarmer. For local development: point Claude Code (or another local
# MCP client) at the locally-running mcpserver
# (`claude mcp add --transport http val-analyzer http://localhost:8092/mcp`)
# without a second discordbot instance competing with the deployed
# production bot for the same Discord Gateway events (see CLAUDE.md's
# "Will the same interaction get delivered to both?" notes). --dev also
# stops discordbot/cachewarmer if they're already running locally from a
# previous full-stack run, since the whole point is that they must not be
# running - just not starting new ones isn't enough.
#
# The discordbot service only starts if the "discordbot" Compose profile is
# active - set COMPOSE_PROFILES=discordbot in .env once DISCORD_BOT_TOKEN
# (and ANTHROPIC_API_KEY) are set, since cmd/discordbot fails fast without
# them.
set -e

BUILD=false
DEV=false
for arg in "$@"; do
  case "$arg" in
    --build|-b) BUILD=true ;;
    --dev) DEV=true ;;
  esac
done

if [ "$DEV" = true ]; then
  echo "DEV mode: polyglot + mcpserver only (no discordbot, no cachewarmer)"
  docker compose stop discordbot cachewarmer >/dev/null 2>&1 || true
  if [ "$BUILD" = true ]; then
    docker compose up -d --build polyglot mcpserver
  else
    docker compose up -d polyglot mcpserver
  fi
else
  if [ "$BUILD" = true ]; then
    docker compose up -d --build
  else
    docker compose up -d
  fi
fi

docker compose ps
