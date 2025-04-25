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

// KafkaProducer интерфейс для отправки сообщений в Kafka
type KafkaProducer interface {
	// Produce отправляет сообщение в указанный топик
	Produce(ctx context.Context, topic string, key string, value []byte) error
}

type DepositProducerI interface {
	// SendDepositEvent отправляет событие о депозите в Kafka
	SendDepositEvent(ctx context.Context, deposit *entity.Deposit, userID string, balanceNew decimal.Decimal, idempotencyKey string) error
}

// DepositProducer занимается отправкой событий депозитов в Kafka
type DepositProducer struct {
	kafkaProducer KafkaProducer
	feedTopic     string
}

// NewDepositProducer создает новый экземпляр DepositProducer
func NewDepositProducer(kafkaProducer KafkaProducer, feedTopic string) *DepositProducer {
	return &DepositProducer{
		kafkaProducer: kafkaProducer,
		feedTopic:     feedTopic,
	}
}

// SendDepositEvent отправляет событие о депозите в Kafka
func (p *DepositProducer) SendDepositEvent(
	ctx context.Context,
	deposit *entity.Deposit,
	userID string,
	balanceNew decimal.Decimal,
	idempotencyKey string,
) error {

	// Преобразование типов в protobuf структуру
	depositOperationEvent := convertDepositToProto(deposit, userID, balanceNew, idempotencyKey)

	// Сериализация protobuf сообщения
	eventBytes, err := proto.Marshal(depositOperationEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal deposit event: %w", err)
	}

	// Используем userID как ключ для сообщения
	key := userID

	logger.Info(ctx, "Sending deposit event to Kafka",
		zap.String("topic", p.feedTopic),
		zap.String("deposit_id", deposit.ID.String()),
		zap.String("account_id", deposit.AccountID.String()),
		zap.String("amount", deposit.Amount.String()),
		zap.String("status", deposit.Status.String()))

	// Отправка сообщения в Kafka
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

// convertToProtoStatus конвертирует внутренний статус депозита в формат для протобаффера
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
