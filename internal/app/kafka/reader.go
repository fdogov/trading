package kafka

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/IBM/sarama"
)

// Reader represents a reader of messages from Kafka
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

// Message represents a message from Kafka
type Message struct {
	Key   []byte
	Value []byte
	Topic string
}

// NewReader creates a new Reader instance
func NewReader(brokers []string, groupID, topic string) *Reader {
	return &Reader{
		groupID:       groupID,
		topic:         topic,
		brokers:       brokers,
		messageCh:     make(chan Message, 100), // Buffer for messages
		shutdownCh:    make(chan struct{}),
		partConsumers: make([]sarama.PartitionConsumer, 0),
		logger:        zap.NewNop(), // Default noop logger, will be replaced at Start
	}
}

// SetLogger sets the logger for the Reader
func (r *Reader) SetLogger(logger *zap.Logger) {
	r.logger = logger.With(zap.String("topic", r.topic))
}

// Start starts reading messages from Kafka
func (r *Reader) Start(ctx context.Context) error {
	// Create Sarama configuration
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.ClientID = r.groupID

	// Create consumer
	consumer, err := sarama.NewConsumer(r.brokers, config)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}
	r.consumer = consumer

	// Get list of topic partitions
	partitions, err := consumer.Partitions(r.topic)
	if err != nil {
		return fmt.Errorf("failed to get partitions for topic %s: %w", r.topic, err)
	}

	// Create a consumer for each partition
	for _, partition := range partitions {
		// Start from the latest offset
		partConsumer, err := consumer.ConsumePartition(r.topic, partition, sarama.OffsetNewest)
		if err != nil {
			return fmt.Errorf("failed to create partition consumer: %w", err)
		}
		r.partConsumers = append(r.partConsumers, partConsumer)

		// Start message handler for the partition
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
					// Send message to channel
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

// Messages returns a channel for reading messages
func (r *Reader) Messages() <-chan Message {
	return r.messageCh
}

// Close closes the reader
func (r *Reader) Close() error {
	close(r.shutdownCh)

	// Close all partition consumers
	for _, partConsumer := range r.partConsumers {
		if err := partConsumer.Close(); err != nil {
			r.logger.Warn("Error closing partition consumer", zap.Error(err))
		}
	}

	// Close consumer
	if r.consumer != nil {
		if err := r.consumer.Close(); err != nil {
			return fmt.Errorf("failed to close Kafka consumer: %w", err)
		}
	}

	// Wait for all goroutines to finish
	r.wg.Wait()
	return nil
}
