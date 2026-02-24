#!/usr/bin/env bash
set -euo pipefail

# AIStarlight Go — GCE deploy script
# Usage: ./deploy/deploy.sh [--migrate-only] [--api-only] [--worker-only]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE="sudo docker compose -p aistarlight -f docker-compose.prod.yml"

echo "==> Deploying AIStarlight Go..."
cd "$PROJECT_DIR"

# Run migrations (one-shot container)
echo "==> Running database migrations..."
$COMPOSE run --rm migrate

if [[ "${1:-}" == "--migrate-only" ]]; then
    echo "==> Migrations complete. Exiting."
    exit 0
fi

# Determine which services to rebuild
SERVICES="api worker"
if [[ "${1:-}" == "--api-only" ]]; then
    SERVICES="api"
elif [[ "${1:-}" == "--worker-only" ]]; then
    SERVICES="worker"
fi

# Build and restart
echo "==> Building images for: $SERVICES..."
$COMPOSE build $SERVICES

echo "==> Starting services..."
$COMPOSE up -d $SERVICES

# Reload nginx to pick up new container IPs
$COMPOSE exec -T nginx nginx -s reload 2>/dev/null || true

# Health check (only if api is being deployed)
if [[ "$SERVICES" == *"api"* ]]; then
    echo "==> Waiting for API health check..."
    for i in $(seq 1 30); do
        if curl -sf http://localhost/health > /dev/null 2>&1; then
            echo "==> API is healthy!"
            break
        fi
        if [ "$i" -eq 30 ]; then
            echo "==> ERROR: API health check failed after 30 seconds"
            $COMPOSE logs api --tail=50
            exit 1
        fi
        sleep 1
    done
fi

# Cleanup old images
echo "==> Cleaning up unused images..."
sudo docker image prune -f

echo "==> Deploy complete!"
$COMPOSE ps
