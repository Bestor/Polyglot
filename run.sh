#!/bin/sh
# Runs the val-analyzer container. Pass --build (or -b) to rebuild the
# image first; otherwise the existing local image is reused.
set -e

IMAGE=val-analyzer
CONTAINER=val-analyzer

BUILD=false
for arg in "$@"; do
  case "$arg" in
    --build|-b) BUILD=true ;;
  esac
done

if [ "$BUILD" = true ]; then
  docker build -t "$IMAGE" .
fi

docker rm -f "$CONTAINER" >/dev/null 2>&1 || true

docker run -d --name "$CONTAINER" \
  -p 8090:8090 \
  --env-file .env \
  -v val-analyzer-data:/app/pb_data \
  "$IMAGE"

echo "val-analyzer running at http://localhost:8090 (docker logs -f $CONTAINER)"
