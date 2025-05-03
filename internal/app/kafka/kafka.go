package kafka

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/domain/accounts"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/domain/orders"
	"github.com/fdogov/trading/internal/producers"
	"github.com/fdogov/trading/internal/store"
)

// Consumer represents an interface for processing messages from Kafka
type Consumer interface {
	ProcessMessage(ctx context.Context, message []byte) error
}

// KafkaConsumers contains all Kafka consumers
type KafkaConsumers struct {
	accountConsumer *accounts.AccountConsumer
	depositConsumer *finance.DepositConsumer
	orderConsumer   *orders.OrderConsumer
	readers         []*Reader
	wg              sync.WaitGroup
	shutdownCh      chan struct{}
	logger          *zap.Logger
}

// NewKafkaConsumers creates a new KafkaConsumers instance
func NewKafkaConsumers(
	accountStore store.AccountStore,
	depositStore store.DepositStore,
	orderStore store.OrderStore,
	eventStore store.EventStore,
	depositProducer *producers.DepositProducer,
	orderProducer *producers.OrderProducer,
	partnerProxyAccountClient dependency.PartnerProxyAccountClient,
	logger *zap.Logger,
) *KafkaConsumers {
	return &KafkaConsumers{
		accountConsumer: accounts.NewAccountConsumer(accountStore, logger),
		depositConsumer: finance.NewDepositConsumer(depositStore, accountStore, eventStore, depositProducer, partnerProxyAccountClient),
		orderConsumer:   orders.NewOrderConsumer(orderStore, accountStore, eventStore, orderProducer),
		readers:         make([]*Reader, 0),
		shutdownCh:      make(chan struct{}),
		logger:          logger,
	}
}

// Start starts all Kafka consumers
func (k *KafkaConsumers) Start(ctx context.Context, cfg config.Kafka) error {
	// Create and start readers for each topic
	accountReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.AccountTopic)
	depositReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.DepositEventTopic)
	orderReader := NewReader(cfg.Brokers, cfg.GroupID, cfg.OrderEventTopic)

	// Set logger for each Reader
	accountReader.SetLogger(k.logger)
	depositReader.SetLogger(k.logger)
	orderReader.SetLogger(k.logger)

	k.readers = append(k.readers, accountReader, depositReader, orderReader)

	// Start reading messages for each reader
	if err := accountReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start account reader: %w", err)
	}
	if err := depositReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start deposit reader: %w", err)
	}
	if err := orderReader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start order reader: %w", err)
	}

	// Start message handlers
	k.startMessageHandler(ctx, accountReader, k.accountConsumer, "account")
	k.startMessageHandler(ctx, depositReader, k.depositConsumer, "deposit")
	k.startMessageHandler(ctx, orderReader, k.orderConsumer, "order")

	k.logger.Info("Kafka consumers started",
		zap.Strings("topics", []string{cfg.AccountTopic, cfg.DepositEventTopic, cfg.OrderEventTopic}))

	return nil
}

// startMessageHandler starts a message handler for a specific reader
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

// Stop stops all Kafka consumers
func (k *KafkaConsumers) Stop() {
	close(k.shutdownCh)

	var wg sync.WaitGroup
	for _, reader := range k.readers {
		wg.Add(1)
		go func(r *Reader) {
			defer wg.Done()
			if err := r.Close(); err != nil {
				k.logger.Error("Error closing Kafka reader", zap.Error(err))
			}
		}(reader)
	}
	wg.Wait()

	// Wait for all goroutines to finish
	k.wg.Wait()
	k.logger.Info("Kafka consumers stopped")
}
