package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Producer представляет клиент для отправки сообщений в Kafka
type Producer struct {
	producer sarama.SyncProducer
	logger   *zap.Logger
}

// NewProducer создает новый экземпляр Producer
func NewProducer(brokers []string, logger *zap.Logger) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll // Ожидание подтверждения от всех реплик
	config.Producer.Retry.Max = 5                    // Количество повторных попыток
	config.Producer.Return.Successes = true          // Нужно для синхронного продюсера

	// Создаем синхронный продюсер
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Producer{
		producer: producer,
		logger:   logger,
	}, nil
}

// Produce отправляет сообщение в указанный топик
func (p *Producer) Produce(ctx context.Context, topic string, key string, value []byte) error {
	// Создаем сообщение
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	// Добавляем ключ, если он предоставлен
	if key != "" {
		msg.Key = sarama.StringEncoder(key)
	}

	// Отправляем сообщение
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

// Close закрывает соединение с Kafka
func (p *Producer) Close() error {
	return p.producer.Close()
}
