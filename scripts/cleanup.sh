#!/bin/bash
# Скрипт для очистки контейнеров Podman

echo "Удаляем контейнеры..."
podman rm -f trading-postgres trading-redpanda trading-redpanda-console trading-redpanda-setup trading-migrations trading-service || true

echo "Удаляем pod..."
podman pod rm -f pod_trading || true

echo "Очистка завершена. Теперь можно заново запустить compose:up"
