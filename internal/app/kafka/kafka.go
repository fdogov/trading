package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/fdogov/trading/internal/domain/accounts"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/domain/orders"
	"go.uber.org/zap"

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/store"
)

// Consumer представляет интерфейс для обработки сообщений из Kafka
type Consumer interface {
	ProcessMessage(ctx context.Context, message []byte) error
}

// KafkaConsumers содержит все консьюмеры Kafka
type KafkaConsumers struct {
	accountConsumer *accounts.AccountConsumer
	depositConsumer *finance.DepositConsumer
	orderConsumer   *orders.OrderConsumer
	readers         []*Reader
	wg              sync.WaitGroup
	shutdownCh      chan struct{}
	logger          *zap.Logger
}

// NewKafkaConsumers создает новый экземпляр KafkaConsumers
func NewKafkaConsumers(
	accountStore store.AccountStore,
	depositStore store.DepositStore,
	orderStore store.OrderStore,
	dbTransactor store.DBTransactor,
	logger *zap.Logger,
) *KafkaConsumers {
	return &KafkaConsumers{
		accountConsumer: accounts.NewAccountConsumer(accountStore, logger),
		depositConsumer: finance.NewDepositConsumer(depositStore, accountStore, dbTransactor),
		orderConsumer:   orders.NewOrderConsumer(orderStore, accountStore, dbTransactor),
		readers:         make([]*Reader, 0),
		shutdownCh:      make(chan struct{}),
		logger:          logger,
	}
}

// Start запускает все консьюмеры Kafka
func (k *KafkaConsumers) Start(ctx context.Context, cfg config.Kafka) error {
	// Создаем и запускаем читателей для каждого топика
	accountReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.AccountTopic)
	depositReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.DepositEventTopic)
	orderReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.OrderEventTopic)

	// Устанавливаем логгер для каждого Reader
	accountReader.SetLogger(k.logger)
	depositReader.SetLogger(k.logger)
	orderReader.SetLogger(k.logger)

	k.readers = append(k.readers, accountReader, depositReader, orderReader)

	// Запускаем чтение сообщений для каждого читателя
	if err := accountReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start account reader: %w", err)
	}
	if err := depositReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start deposit reader: %w", err)
	}
	if err := orderReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start order reader: %w", err)
	}

	// Запускаем обработчики сообщений
	k.startMessageHandler(ctx, accountReader, k.accountConsumer, "account")
	k.startMessageHandler(ctx, depositReader, k.depositConsumer, "deposit")
	k.startMessageHandler(ctx, orderReader, k.orderConsumer, "order")

	k.logger.Info("Kafka consumers started",
		zap.Strings("topics", []string{cfg.AccountTopic, cfg.DepositEventTopic, cfg.OrderEventTopic}))

	return nil
}

// startMessageHandler запускает обработчик сообщений для конкретного читателя
func (k *KafkaConsumers) startMessageHandler(ctx context.Context, reader *Reader, consumer Consumer, consumerName string) {
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-k.shutdownCh:
				return
			case msg := <-reader.Messages():
				err := consumer.ProcessMessage(ctx, msg.Value)
				if err != nil {
					k.logger.Error("Error processing message",
						zap.String("consumer", consumerName),
						zap.Error(err))
				}
			}
		}
	}()
}

// Stop останавливает все консьюмеры Kafka
func (k *KafkaConsumers) Stop() {
	close(k.shutdownCh)

	// Закрываем все читатели
	for _, reader := range k.readers {
		if err := reader.Close(); err != nil {
			k.logger.Error("Error closing Kafka reader", zap.Error(err))
		}
	}

	// Ждем завершения всех горутин
	k.wg.Wait()
	k.logger.Info("Kafka consumers stopped")
}
