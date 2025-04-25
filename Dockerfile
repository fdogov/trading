FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/trading ./cmd/trading

# Final image
FROM alpine:3.18

WORKDIR /app

# Set up necessary packages
RUN apk --no-cache add ca-certificates tzdata postgresql-client netcat-openbsd

# Copy binary file from builder
COPY --from=builder /app/trading /app/trading
COPY --from=builder /app/migrations /app/migrations

# Copy wait scripts
COPY --from=builder /app/scripts/wait-for-postgres.sh /app/wait-for-postgres.sh
COPY --from=builder /app/scripts/wait-for-kafka.sh /app/wait-for-redpanda.sh
RUN chmod +x /app/wait-for-postgres.sh /app/wait-for-redpanda.sh

# Set environment variables
ENV GRPC_PORT=50051
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=postgres
ENV DB_PASSWORD=postgres
ENV DB_NAME=trading
ENV DB_SSLMODE=disable
ENV PARTNER_PROXY_ADDRESS=partner-proxy:50051
ENV KAFKA_BROKER=redpanda:9092
ENV KAFKA_GROUP_ID=trading
ENV KAFKA_ACCOUNT_TOPIC=origination.account
ENV KAFKA_DEPOSIT_EVENT_TOPIC=partnerconsumer.deposit
ENV KAFKA_ORDER_EVENT_TOPIC=partnerconsumer.order

# Run application
CMD /app/wait-for-postgres.sh postgres && /app/wait-for-redpanda.sh redpanda && /app/trading
