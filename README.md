# Trading Service

Service for managing trading accounts, orders and financial operations.

## Architecture

The service implements the following gRPC APIs:
- `AccountService` - trading account management
- `OrderService` - order creation and management
- `FinanceService` - financial operations (deposits)

The service interacts with:
- `PartnerProxy` - for proxying orders and financial operations to the partner
- Redpanda - for receiving events from other services (compatible with Kafka protocol)
- PostgreSQL - for data storage

## Project Structure

```
trading/
├── cmd/               # Executable files
│   └── trading/       # Main executable file
├── docker-compose.yml # File for running the service with PostgreSQL and Redpanda
├── internal/          # Internal packages
│   ├── app/           # Application and its components
│   │   ├── accounts/  # API handlers for accounts
│   │   ├── finance/   # API handlers for finances
│   │   ├── kafka/     # Kafka/Redpanda handlers
│   │   └── orders/    # API handlers for orders
│   ├── config/        # Configuration
│   ├── dependency/    # Clients for external services
│   ├── entity/        # Domain models
│   └── store/         # Storage interfaces
│       └── postgres/  # PostgreSQL implementation of storages
├── migrations/        # SQL migrations for PostgreSQL
├── scripts/           # Helper scripts
├── Taskfile.yml       # Task file for automation
└── tools/             # Helper tools
    └── kafka-producer/ # Utility for sending test messages
```

## Domain Model

### Account
Represents a user's trading account with balance and status.

### Order
Represents an order to buy or sell an instrument.

### Deposit
Represents a deposit operation to an account.

## Database

The service uses PostgreSQL for data storage. The database structure is described in SQL migrations in the `migrations/` directory.

## Redpanda (Kafka-compatible broker)

The service uses Redpanda to receive events from other services:
- `origination.account` - account creation events
- `partnerconsumer.deposit` - deposit status update events
- `partnerconsumer.order` - order status update events

Redpanda provides a web interface (Redpanda Console) for viewing topics and messages, available at http://localhost:8080.

## Running using Task

The project uses [Task](https://taskfile.dev/) for automating various commands. First install Task, then use the following commands:

```bash
# View available tasks
task

# Run the project
task docker:up

# Run migrations
task docker:migration:up

# View logs
task docker:logs:trading

# Send test events
task kafka:send:account
task kafka:send:deposit
task kafka:send:order

# Stop the project
task docker:down
```

## Running using Docker Compose

```bash
# Running service with PostgreSQL and Redpanda
docker-compose up -d

# View logs
docker-compose logs -f trading

# Access Redpanda Console web interface
# Open in browser http://localhost:8080
```

## Local running

```bash
# Create and migrate database
psql -c "CREATE DATABASE trading;"
migrate -source file://migrations -database "postgres://postgres:postgres@localhost:5432/trading?sslmode=disable" up

# Run service
go run cmd/trading/main.go
```

## Testing with Redpanda

For sending test messages, you can use the kafka-producer utility (it works with Redpanda too):

```bash
# Send account creation event
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=account

# Send deposit event
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=deposit

# Send order event
go run tools/kafka-producer/main.go -broker=localhost:9092 -type=order
```

You can also use Redpanda Console to view topics and messages:
- Open in browser http://localhost:8080
- Go to "Topics" section
- Select the required topic and view messages

## Environment Variables

- `GRPC_PORT` - port for gRPC server (default "50051")
- `PARTNER_PROXY_ADDRESS` - PartnerProxy service address (default "localhost:50052")
- `KAFKA_BROKER` - Redpanda broker address (default "localhost:9092")
- `KAFKA_GROUP_ID` - group ID (default "trading")
- `KAFKA_ACCOUNT_TOPIC` - topic for account events (default "origination.account")
- `KAFKA_DEPOSIT_EVENT_TOPIC` - topic for deposit events (default "partnerconsumer.deposit")
- `KAFKA_ORDER_EVENT_TOPIC` - topic for order events (default "partnerconsumer.order")
- `DB_HOST` - database host (default "localhost")
- `DB_PORT` - database port (default "5432")
- `DB_USER` - database user (default "postgres")
- `DB_PASSWORD` - database password (default "postgres")
- `DB_NAME` - database name (default "trading")
- `DB_SSLMODE` - SSL mode for database (default "disable")
