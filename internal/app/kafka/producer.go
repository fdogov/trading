package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Producer represents a client for sending messages to Kafka
type Producer struct {
	producer sarama.SyncProducer
	logger   *zap.Logger
}

// NewProducer creates a new Producer instance
func NewProducer(brokers []string, logger *zap.Logger) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll // Wait for confirmation from all replicas
	config.Producer.Retry.Max = 5                    // Number of retry attempts
	config.Producer.Return.Successes = true          // Required for synchronous producer

	// Create synchronous producer
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Producer{
		producer: producer,
		logger:   logger,
	}, nil
}

// Produce sends a message to the specified topic
func (p *Producer) Produce(ctx context.Context, topic string, key string, value []byte) error {
	// Create message
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	// Add key if provided
	if key != "" {
		msg.Key = sarama.StringEncoder(key)
	}

	// Send message
	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.Error("Failed to send message to Kafka",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to send message to Kafka: %w", err)
	}

	p.logger.Debug("Message sent to Kafka",
		zap.String("topic", topic),
		zap.String("key", key),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
		zap.Int("value_size", len(value)))

	return nil
}

// Close closes the connection to Kafka
func (p *Producer) Close() error {
	return p.producer.Close()
}
