#!/bin/bash
# Script for cleaning up Podman containers

echo "Removing containers..."
podman rm -f trading-postgres trading-redpanda trading-redpanda-console trading-redpanda-setup trading-migrations trading-service || true

echo "Removing pod..."
podman pod rm -f pod_trading || true

echo "Cleanup completed. Now you can restart compose:up"
