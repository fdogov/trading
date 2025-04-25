package kafka

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/IBM/sarama"
)

// Reader представляет читателя сообщений из Kafka
type Reader struct {
	consumer      sarama.Consumer
	partConsumers []sarama.PartitionConsumer
	groupID       string
	topic         string
	brokers       []string
	messageCh     chan Message
	shutdownCh    chan struct{}
	wg            sync.WaitGroup
	logger        *zap.Logger
}

// Message представляет сообщение из Kafka
type Message struct {
	Key   []byte
	Value []byte
	Topic string
}

// NewReader создает новый экземпляр Reader
func NewReader(brokers []string, groupID, topic string) *Reader {
	return &Reader{
		groupID:       groupID,
		topic:         topic,
		brokers:       brokers,
		messageCh:     make(chan Message, 100), // Буфер для сообщений
		shutdownCh:    make(chan struct{}),
		partConsumers: make([]sarama.PartitionConsumer, 0),
		logger:        zap.NewNop(), // По умолчанию noop логгер, будет заменен при Start
	}
}

// SetLogger устанавливает логгер для Reader
func (r *Reader) SetLogger(logger *zap.Logger) {
	r.logger = logger.With(zap.String("topic", r.topic))
}

// Start запускает чтение сообщений из Kafka
func (r *Reader) Start(ctx context.Context) error {
	// Создаем конфигурацию Sarama
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.ClientID = r.groupID

	// Создаем потребителя
	consumer, err := sarama.NewConsumer(r.brokers, config)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}
	r.consumer = consumer

	// Получаем список разделов топика
	partitions, err := consumer.Partitions(r.topic)
	if err != nil {
		return fmt.Errorf("failed to get partitions for topic %s: %w", r.topic, err)
	}

	// Создаем потребитель для каждого раздела
	for _, partition := range partitions {
		// Начинаем с последнего смещения
		partConsumer, err := consumer.ConsumePartition(r.topic, partition, sarama.OffsetNewest)
		if err != nil {
			return fmt.Errorf("failed to create partition consumer: %w", err)
		}
		r.partConsumers = append(r.partConsumers, partConsumer)

		// Запускаем обработчик сообщений из раздела
		r.wg.Add(1)
		go func(pc sarama.PartitionConsumer) {
			defer r.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-r.shutdownCh:
					return
				case err := <-pc.Errors():
					r.logger.Error("Error reading from Kafka", zap.Error(err))
				case m := <-pc.Messages():
					// Отправляем сообщение в канал
					r.messageCh <- Message{
						Key:   m.Key,
						Value: m.Value,
						Topic: m.Topic,
					}
				}
			}
		}(partConsumer)
	}

	return nil
}

// Messages возвращает канал для чтения сообщений
func (r *Reader) Messages() <-chan Message {
	return r.messageCh
}

// Close закрывает читателя
func (r *Reader) Close() error {
	close(r.shutdownCh)

	// Закрываем все консьюмеры разделов
	for _, partConsumer := range r.partConsumers {
		if err := partConsumer.Close(); err != nil {
			r.logger.Warn("Error closing partition consumer", zap.Error(err))
		}
	}

	// Закрываем консьюмера
	if r.consumer != nil {
		if err := r.consumer.Close(); err != nil {
			return fmt.Errorf("failed to close Kafka consumer: %w", err)
		}
	}

	// Ждем завершения всех горутин
	r.wg.Wait()
	return nil
}
