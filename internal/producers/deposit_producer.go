package producers

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	feedkafkav1 "github.com/fdogov/contracts/gen/go/backend/feed/kafka/v1"
	googletypedecimal "github.com/fdogov/contracts/gen/go/google/type"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
)

// KafkaProducer interface for sending messages to Kafka
type KafkaProducer interface {
	// Produce sends a message to the specified topic
	Produce(ctx context.Context, topic string, key string, value []byte) error
}

type DepositProducerI interface {
	// SendDepositEvent sends a deposit event to Kafka
	SendDepositEvent(ctx context.Context, deposit *entity.Deposit, userID string, balanceNew decimal.Decimal, idempotencyKey string) error
}

// DepositProducer handles sending deposit events to Kafka
type DepositProducer struct {
	kafkaProducer KafkaProducer
	feedTopic     string
}

// NewDepositProducer creates a new DepositProducer instance
func NewDepositProducer(kafkaProducer KafkaProducer, feedTopic string) *DepositProducer {
	return &DepositProducer{
		kafkaProducer: kafkaProducer,
		feedTopic:     feedTopic,
	}
}

// SendDepositEvent sends a deposit event to Kafka
func (p *DepositProducer) SendDepositEvent(
	ctx context.Context,
	deposit *entity.Deposit,
	userID string,
	balanceNew decimal.Decimal,
	idempotencyKey string,
) error {

	// Converting types to protobuf structure
	depositOperationEvent := convertDepositToProto(deposit, userID, balanceNew, idempotencyKey)

	// Serializing protobuf message
	eventBytes, err := proto.Marshal(depositOperationEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal deposit event: %w", err)
	}

	// Using userID as the message key
	key := userID

	logger.Info(ctx, "Sending deposit event to Kafka",
		zap.String("topic", p.feedTopic),
		zap.String("deposit_id", deposit.ID.String()),
		zap.String("account_id", deposit.AccountID.String()),
		zap.String("amount", deposit.Amount.String()),
		zap.String("status", deposit.Status.String()))

	// Sending the message to Kafka
	err = p.kafkaProducer.Produce(ctx, p.feedTopic, key, eventBytes)
	if err != nil {
		return fmt.Errorf("failed to produce deposit event to Kafka: %w", err)
	}

	return nil
}

func convertDepositToProto(
	deposit *entity.Deposit,
	userID string,
	balanceNew decimal.Decimal,
	idempotencyKey string,
) *feedkafkav1.DepositOperationEvent {
	return &feedkafkav1.DepositOperationEvent{
		Id:        deposit.ID.String(),
		UserId:    userID,
		AccountId: deposit.AccountID.String(),
		Currency:  deposit.Currency,
		Amount: &googletypedecimal.Decimal{
			Value: deposit.Amount.String(),
		},
		BalanceNew: &googletypedecimal.Decimal{
			Value: balanceNew.String(),
		},
		Status:         convertToProtoStatus(deposit.Status),
		CreatedAt:      timestamppb.New(deposit.CreatedAt),
		IdempotencyKey: idempotencyKey,
	}
}

// convertToProtoStatus converts internal deposit status to protobuf format
func convertToProtoStatus(status entity.DepositStatus) feedkafkav1.DepositOperationStatus {
	switch status {
	case entity.DepositStatusPending:
		return feedkafkav1.DepositOperationStatus_DEPOSIT_OPERATION_STATUS_PENDING
	case entity.DepositStatusCompleted:
		return feedkafkav1.DepositOperationStatus_DEPOSIT_OPERATION_STATUS_COMPLETED
	case entity.DepositStatusFailed:
		return feedkafkav1.DepositOperationStatus_DEPOSIT_OPERATION_STATUS_FAILED
	default:
		return feedkafkav1.DepositOperationStatus_DEPOSIT_OPERATION_STATUS_UNSPECIFIED
	}
}
