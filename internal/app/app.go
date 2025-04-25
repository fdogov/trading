package app

import (
	"context"
	"fmt"
	"net"

	"github.com/fdogov/trading/internal/domain/accounts"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/domain/orders"
	"github.com/fdogov/trading/internal/producers"
	"go.uber.org/zap"

	"google.golang.org/grpc"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/app/kafka"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/store"
)

// App represents the Trading application
type App struct {
	cfg                       config.Config
	grpcServer                *grpc.Server
	accountsServer            *accounts.Server
	ordersServer              *orders.Server
	financeServer             *finance.Server
	kafkaConsumers            *kafka.KafkaConsumers
	kafkaProducer             *kafka.Producer
	partnerProxyOrderClient   dependency.PartnerProxyOrderClient
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient
	logger                    *zap.Logger
}

// NewApp creates a new App instance
func NewApp(
	cfg config.Config,
	accountStore store.AccountStore,
	orderStore store.OrderStore,
	depositStore store.DepositStore,
	eventStore store.EventStore,
	dbTransactor store.DBTransactor,
	logger *zap.Logger,
) (*App, error) {
	// Create clients for partner services
	partnerProxyOrderClient, err := dependency.NewPartnerProxyOrderClient(cfg.Dependencies.PartnerProxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner proxy order client: %w", err)
	}

	partnerProxyFinanceClient, err := dependency.NewPartnerProxyFinanceClient(cfg.Dependencies.PartnerProxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner proxy finance client: %w", err)
	}

	// Create Kafka Producer
	kafkaProducer, err := kafka.NewProducer(cfg.Kafka.Brokers, logger.Named("kafka-producer"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create DepositProducer
	depositProducer := producers.NewDepositProducer(kafkaProducer, cfg.Kafka.DepositFeedTopic)

	// Create gRPC servers
	accountsServer := accounts.NewServer(accountStore)
	ordersServer := orders.NewServer(orderStore, accountStore, dbTransactor, partnerProxyOrderClient)
	financeServer := finance.NewServer(depositStore, accountStore, partnerProxyFinanceClient, logger)

	// Create Kafka consumers
	kafkaConsumers := kafka.NewKafkaConsumers(
		accountStore,
		depositStore,
		orderStore,
		eventStore,
		depositProducer,
		dbTransactor,
		logger.Named("kafka-consumer"),
	)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	tradingv1.RegisterAccountServiceServer(grpcServer, accountsServer)
	tradingv1.RegisterOrderServiceServer(grpcServer, ordersServer)
	tradingv1.RegisterFinanceServiceServer(grpcServer, financeServer)

	return &App{
		cfg:                       cfg,
		grpcServer:                grpcServer,
		accountsServer:            accountsServer,
		ordersServer:              ordersServer,
		financeServer:             financeServer,
		kafkaConsumers:            kafkaConsumers,
		kafkaProducer:             kafkaProducer,
		partnerProxyOrderClient:   partnerProxyOrderClient,
		partnerProxyFinanceClient: partnerProxyFinanceClient,
		logger:                    logger,
	}, nil
}

// Start starts the application
func (a *App) Start(ctx context.Context) error {
	// Start Kafka consumers
	if err := a.kafkaConsumers.Start(ctx, a.cfg.Kafka); err != nil {
		return fmt.Errorf("failed to start Kafka consumers: %w", err)
	}

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", a.cfg.GRPC.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.logger.Info("Starting gRPC server", zap.String("port", a.cfg.GRPC.Port))
	return a.grpcServer.Serve(lis)
}

// Stop stops the application
func (a *App) Stop() {
	// Stop Kafka consumers
	a.kafkaConsumers.Stop()

	// Close Kafka producer
	if err := a.kafkaProducer.Close(); err != nil {
		a.logger.Error("Error closing Kafka producer", zap.Error(err))
	}

	// Stop gRPC server
	a.grpcServer.GracefulStop()

	a.logger.Info("Application stopped")
}
