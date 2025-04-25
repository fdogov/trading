# Trading Service

Сервис для управления торговыми аккаунтами, заказами и финансовыми операциями.

## Архитектура

Сервис реализует следующие gRPC API:
- `AccountService` - управление торговыми аккаунтами
- `OrderService` - создание и управление заказами
- `FinanceService` - финансовые операции (депозиты)

Сервис взаимодействует с:
- `PartnerProxy` - для проксирования заказов и финансовых операций к партнеру
- Redpanda - для получения событий от других сервисов (совместим с протоколом Kafka)
- PostgreSQL - для хранения данных

## Структура проекта

```
trading/
├── cmd/               # Исполняемые файлы
│   └── trading/       # Главный исполняемый файл
├── docker-compose.yml # Файл для запуска сервиса с PostgreSQL и Redpanda
├── internal/          # Внутренние пакеты
│   ├── app/           # Приложение и его компоненты
│   │   ├── accounts/  # Обработчики API аккаунтов
│   │   ├── finance/   # Обработчики API финансов
│   │   ├── kafka/     # Обработчики Kafka/Redpanda
│   │   └── orders/    # Обработчики API заказов
│   ├── config/        # Конфигурация
│   ├── dependency/    # Клиенты для внешних сервисов
│   ├── entity/        # Доменные модели
│   └── store/         # Интерфейсы хранилищ
│       └── postgres/  # Реализация хранилищ для PostgreSQL
├── migrations/        # SQL-миграции для PostgreSQL
├── scripts/           # Вспомогательные скрипты
├── Taskfile.yml       # Файл задач для автоматизации
└── tools/             # Вспомогательные инструменты
    └── kafka-producer/ # Утилита для отправки тестовых сообщений
```

## Доменная модель

### Account
Представляет торговый аккаунт пользователя с балансом и статусом.

### Order
Представляет заказ на покупку или продажу инструмента.

### Deposit
Представляет операцию внесения средств на аккаунт.

## База данных

Сервис использует PostgreSQL для хранения данных. Структура базы данных описана в SQL-миграциях в директории `migrations/`.

## Redpanda (Kafka-совместимый брокер)

Сервис использует Redpanda для получения событий от других сервисов:
- `origination.account` - события создания аккаунтов
- `partnerconsumer.deposit` - события обновления статусов депозитов
- `partnerconsumer.order` - события обновления статусов заказов

Redpanda предоставляет веб-интерфейс (Redpanda Console) для просмотра топиков и сообщений, доступный по адресу http://localhost:8080.

## Запуск с использованием Task

Проект использует [Task](https://taskfile.dev/) для автоматизации различных команд. Сначала установите Task, затем используйте следующие команды:

```bash
# Просмотр доступных задач
task

# Запуск проекта
task docker:up

# Запуск миграций
task docker:migration:up

# Просмотр логов
task docker:logs:trading

# Отправка тестовых событий
task kafka:send:account
task kafka:send:deposit
task kafka:send:order

# Остановка проекта
task docker:down
```

## Запуск с использованием Docker Compose

```bash
# Запуск сервиса с PostgreSQL и Redpanda
docker-compose up -d

# Просмотр логов
docker-compose logs -f trading

# Доступ к веб-интерфейсу Redpanda Console
# Откройте в браузере http://localhost:8080
```

## Локальный запуск

```bash
# Создание и миграция базы данных
psql -c "CREATE DATABASE trading;"
migrate -source file://migrations -database "postgres://postgres:postgres@localhost:5432/trading?sslmode=disable" up

# Запуск сервиса
go run cmd/trading/main.go
```

## Тестирование с Redpanda

Для отправки тестовых сообщений можно использовать утилиту kafka-producer (она работает и с Redpanda):

```bash
# Отправка события создания аккаунта
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=account

# Отправка события депозита
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=deposit

# Отправка события заказа
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=order
```

Также можно использовать Redpanda Console для просмотра топиков и сообщений:
- Откройте в браузере http://localhost:8080
- Перейдите в раздел "Topics"
- Выберите нужный топик и просмотрите сообщения

## Переменные окружения

- `GRPC_PORT` - порт для gRPC-сервера (по умолчанию "50051")
- `PARTNER_PROXY_ADDRESS` - адрес сервиса PartnerProxy (по умолчанию "localhost:50052")
- `KAFKA_BROKER` - адрес Redpanda брокера (по умолчанию "localhost:9092")
- `KAFKA_GROUP_ID` - ID группы (по умолчанию "trading")
- `KAFKA_ACCOUNT_TOPIC` - топик для событий аккаунтов (по умолчанию "origination.account")
- `KAFKA_DEPOSIT_EVENT_TOPIC` - топик для событий депозитов (по умолчанию "partnerconsumer.deposit")
- `KAFKA_ORDER_EVENT_TOPIC` - топик для событий заказов (по умолчанию "partnerconsumer.order")
- `DB_HOST` - хост базы данных (по умолчанию "localhost")
- `DB_PORT` - порт базы данных (по умолчанию "5432")
- `DB_USER` - пользователь базы данных (по умолчанию "postgres")
- `DB_PASSWORD` - пароль базы данных (по умолчанию "postgres")
- `DB_NAME` - имя базы данных (по умолчанию "trading")
- `DB_SSLMODE` - режим SSL для базы данных (по умолчанию "disable")
