package app

import (
	"context"
	"fmt"
	"net"

	"github.com/fdogov/trading/internal/domain/accounts"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/domain/orders"
	"go.uber.org/zap"

	"google.golang.org/grpc"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/app/kafka"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/store"
)

// App представляет приложение Trading
type App struct {
	cfg                       config.Config
	grpcServer                *grpc.Server
	accountsServer            *accounts.Server
	ordersServer              *orders.Server
	financeServer             *finance.Server
	kafkaConsumers            *kafka.KafkaConsumers
	partnerProxyOrderClient   dependency.PartnerProxyOrderClient
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient
	logger                    *zap.Logger
}

// NewApp создает новый экземпляр App
func NewApp(
	cfg config.Config,
	accountStore store.AccountStore,
	orderStore store.OrderStore,
	depositStore store.DepositStore,
	dbTransactor store.DBTransactor,
	logger *zap.Logger,
) (*App, error) {
	// Создаем клиенты для партнерских сервисов
	partnerProxyOrderClient, err := dependency.NewPartnerProxyOrderClient(cfg.Dependencies.PartnerProxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner proxy order client: %w", err)
	}

	partnerProxyFinanceClient, err := dependency.NewPartnerProxyFinanceClient(cfg.Dependencies.PartnerProxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner proxy finance client: %w", err)
	}

	// Создаем gRPC серверы
	accountsServer := accounts.NewServer(accountStore)
	ordersServer := orders.NewServer(orderStore, accountStore, dbTransactor, partnerProxyOrderClient)
	financeServer := finance.NewServer(depositStore, accountStore, partnerProxyFinanceClient)

	// Создаем Kafka консьюмеры
	kafkaConsumers := kafka.NewKafkaConsumers(
		accountStore,
		depositStore,
		orderStore,
		dbTransactor,
		logger.Named("kafka"),
	)

	// Создаем gRPC сервер
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
		partnerProxyOrderClient:   partnerProxyOrderClient,
		partnerProxyFinanceClient: partnerProxyFinanceClient,
		logger:                    logger,
	}, nil
}

// Start запускает приложение
func (a *App) Start(ctx context.Context) error {
	// Запускаем Kafka консьюмеры
	if err := a.kafkaConsumers.Start(ctx, a.cfg.Kafka); err != nil {
		return fmt.Errorf("failed to start Kafka consumers: %w", err)
	}

	// Запускаем gRPC сервер
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", a.cfg.GRPC.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.logger.Info("Starting gRPC server", zap.String("port", a.cfg.GRPC.Port))
	return a.grpcServer.Serve(lis)
}

// Stop останавливает приложение
func (a *App) Stop() {
	// Останавливаем Kafka консьюмеры
	a.kafkaConsumers.Stop()

	// Останавливаем gRPC сервер
	a.grpcServer.GracefulStop()

	a.logger.Info("Application stopped")
}
