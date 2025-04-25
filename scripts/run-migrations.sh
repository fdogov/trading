#!/bin/bash
# Скрипт для запуска миграций внутри Docker-контейнера

set -e

POSTGRES_HOST=${DB_HOST:-postgres}
POSTGRES_PORT=${DB_PORT:-5432}
POSTGRES_USER=${DB_USER:-postgres}
POSTGRES_PASSWORD=${DB_PASSWORD:-postgres}
POSTGRES_DB=${DB_NAME:-trading}

# Проверяем доступность PostgreSQL
until pg_isready -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER; do
  >&2 echo "PostgreSQL недоступен - ожидаем"
  sleep 1
done

# Пытаемся создать БД (игнорируем ошибку, если БД уже существует)
PGPASSWORD=$POSTGRES_PASSWORD psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -c "CREATE DATABASE $POSTGRES_DB;" || true

# Запускаем миграции
echo "Запускаем миграции..."
migrate -path=/app/migrations -database "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable" up

echo "Миграции выполнены успешно"
