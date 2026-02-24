#!/usr/bin/env bash
set -euo pipefail

# AIStarlight Go — GCE deploy script
# Usage: ./deploy/deploy.sh [--migrate-only]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Building and deploying AIStarlight Go..."

cd "$PROJECT_DIR"

# Run migrations first (one-shot container)
echo "==> Running database migrations..."
docker compose -f docker-compose.prod.yml run --rm migrate

if [[ "${1:-}" == "--migrate-only" ]]; then
    echo "==> Migrations complete. Exiting."
    exit 0
fi

# Build and restart services
echo "==> Building images..."
docker compose -f docker-compose.prod.yml build api worker

echo "==> Starting services..."
docker compose -f docker-compose.prod.yml up -d api worker ocr nginx frontend

# Wait for health check
echo "==> Waiting for API health check..."
for i in $(seq 1 30); do
    if curl -sf http://localhost:8000/health > /dev/null 2>&1; then
        echo "==> API is healthy!"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "==> ERROR: API health check failed after 30 seconds"
        docker compose -f docker-compose.prod.yml logs api --tail=50
        exit 1
    fi
    sleep 1
done

# Cleanup old images
echo "==> Cleaning up unused images..."
docker image prune -f

echo "==> Deploy complete!"
docker compose -f docker-compose.prod.yml ps
