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

//go:generate moq -rm -out gen/order_producer_mock.go -pkg produsersgen -fmt goimports . OrderProducerI

type OrderProducerI interface {
	// SendOrderEvent sends an order event to Kafka
	SendOrderEvent(ctx context.Context, order *entity.Order, balanceNew decimal.Decimal) error
}

// OrderProducer handles sending order events to Kafka
type OrderProducer struct {
	kafkaProducer KafkaProducer
	feedTopic     string
}

// NewOrderProducer creates a new OrderProducer instance
func NewOrderProducer(kafkaProducer KafkaProducer, feedTopic string) *OrderProducer {
	return &OrderProducer{
		kafkaProducer: kafkaProducer,
		feedTopic:     feedTopic,
	}
}

// SendOrderEvent sends an order event to Kafka
func (p *OrderProducer) SendOrderEvent(
	ctx context.Context,
	order *entity.Order,
	balanceNew decimal.Decimal,
) error {
	// Converting types to protobuf structure
	orderOperationEvent := convertOrderToProto(order, balanceNew)

	// Serializing protobuf message
	eventBytes, err := proto.Marshal(orderOperationEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal order event: %w", err)
	}

	// Using userID as the message key
	key := order.UserID

	logger.Info(ctx, "Sending order event to Kafka",
		zap.String("topic", p.feedTopic),
		zap.String("order_id", order.ID.String()),
		zap.String("account_id", order.AccountID.String()),
		zap.String("amount", order.Amount.String()),
		zap.String("status", string(order.Status)))

	// Sending the message to Kafka
	err = p.kafkaProducer.Produce(ctx, p.feedTopic, key, eventBytes)
	if err != nil {
		return fmt.Errorf("failed to produce order event to Kafka: %w", err)
	}

	return nil
}

func convertOrderToProto(
	order *entity.Order,
	balanceNew decimal.Decimal,
) *feedkafkav1.OrderOperationEvent {
	return &feedkafkav1.OrderOperationEvent{
		Id:        order.ID.String(),
		UserId:    order.UserID,
		AccountId: order.AccountID.String(),
		Amount: &googletypedecimal.Decimal{
			Value: order.Amount.String(),
		},
		BalanceNew: &googletypedecimal.Decimal{
			Value: balanceNew.String(),
		},
		Status:         convertToOrderProtoStatus(order.Status),
		CreatedAt:      timestamppb.New(order.CreatedAt),
		Symbol:         order.InstrumentID,
		InstrumentId:   order.InstrumentID,
		OperationType:  feedkafkav1.OrderOperationType_ORDER_OPERATION_TYPE_UPDATE,
		Side:           convertToOrderProtoSide(order.Side),
		IdempotencyKey: order.IdempotencyKey,
	}
}

// convertToOrderProtoStatus converts internal order status to protobuf format
func convertToOrderProtoStatus(status entity.OrderStatus) feedkafkav1.OrderOperationStatus {
	switch status {
	case entity.OrderStatusNew, entity.OrderStatusProcessing:
		return feedkafkav1.OrderOperationStatus_ORDER_OPERATION_STATUS_PENDING
	case entity.OrderStatusCompleted:
		return feedkafkav1.OrderOperationStatus_ORDER_OPERATION_STATUS_COMPLETED
	case entity.OrderStatusFailed:
		return feedkafkav1.OrderOperationStatus_ORDER_OPERATION_STATUS_FAILED
	case entity.OrderStatusCancelled:
		return feedkafkav1.OrderOperationStatus_ORDER_OPERATION_STATUS_CANCELLED
	default:
		return feedkafkav1.OrderOperationStatus_ORDER_OPERATION_STATUS_UNSPECIFIED
	}
}

// convertToOrderProtoSide converts internal order side to protobuf format
func convertToOrderProtoSide(side entity.OrderSide) feedkafkav1.OrderSide {
	switch side {
	case entity.OrderSideBuy:
		return feedkafkav1.OrderSide_ORDER_SIDE_BUY
	case entity.OrderSideSell:
		return feedkafkav1.OrderSide_ORDER_SIDE_SELL
	default:
		return feedkafkav1.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}
