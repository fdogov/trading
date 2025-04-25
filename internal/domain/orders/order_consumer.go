package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store"
)

// OrderConsumer обрабатывает события заказов из Kafka
type OrderConsumer struct {
	orderStore   store.OrderStore
	accountStore store.AccountStore
	dbTransactor store.DBTransactor
}

// NewOrderConsumer создает новый экземпляр OrderConsumer
func NewOrderConsumer(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
) *OrderConsumer {
	return &OrderConsumer{
		orderStore:   orderStore,
		accountStore: accountStore,
		dbTransactor: dbTransactor,
	}
}

// ProcessMessage обрабатывает сообщение из Kafka
func (c *OrderConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	var event partnerconsumerkafkav1.OrderEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("failed to unmarshal order event: %w", err)
	}

	// Добавляем trace ID из внешнего ID события
	ctx = logger.ContextWithTraceID(ctx, event.ExtId)

	logger.Info(ctx, "Received order event",
		zap.String("ext_account_id", event.ExtAccountId),
		zap.String("status", event.Status.String()),
		zap.String("symbol", event.Symbol))

	// Получаем заказ по внешнему ID
	order, err := c.orderStore.GetByExtID(ctx, event.ExtId)
	if err != nil {
		if err != entity.ErrNotFound {
			return fmt.Errorf("failed to get order by ext ID: %w", err)
		}

		// Если заказ не найден, возможно он был создан извне нашей системы
		// Создаем запись о заказе в нашей системе

		// Сначала получаем аккаунт по external account ID
		account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountId)
		if err != nil {
			return fmt.Errorf("failed to get account by ext ID: %w", err)
		}

		// Обновляем контекст с user_id
		ctx = logger.ContextWithUserID(ctx, account.UserID)

		// Преобразуем статус
		var status entity.OrderStatus
		switch event.Status {
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_PENDING:
			status = entity.OrderStatusProcessing
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_COMPLETED:
			status = entity.OrderStatusCompleted
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_FAILED:
			status = entity.OrderStatusFailed
		default:
			status = entity.OrderStatusNew
		}

		// Преобразуем сумму
		amount, err := decimal.NewFromString(event.Amount.Value)
		if err != nil {
			return fmt.Errorf("failed to parse amount: %w", err)
		}

		// Создаем заказ
		now := time.Now()
		order = &entity.Order{
			ID:           uuid.New(),
			UserID:       account.UserID,
			AccountID:    account.ID,
			InstrumentID: event.Symbol,
			Amount:       amount,
			Quantity:     decimal.Zero,           // Пока не знаем точное количество
			OrderType:    entity.OrderTypeMarket, // Предполагаем, что это рыночный заказ
			Side:         entity.OrderSideBuy,    // Предполагаем сторону (может уточнить позже)
			Status:       status,
			Description:  fmt.Sprintf("Order for %s (ext_id: %s)", event.Symbol, event.ExtId),
			ExtID:        event.ExtId,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		err = c.orderStore.Create(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		logger.Info(ctx, "Created new order record",
			zap.String("id", order.ID.String()),
			zap.String("account_id", order.AccountID.String()),
			zap.String("instrument", order.InstrumentID),
			zap.String("amount", order.Amount.String()),
			zap.String("status", string(order.Status)))

		return nil
	}

	// Если заказ найден, обновляем его статус

	// Преобразуем статус из события
	var status entity.OrderStatus
	switch event.Status {
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_PENDING:
		status = entity.OrderStatusProcessing
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_COMPLETED:
		status = entity.OrderStatusCompleted
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_FAILED:
		status = entity.OrderStatusFailed
	default:
		status = entity.OrderStatusNew
	}

	// Если статус не изменился, ничего не делаем
	if order.Status == status {
		logger.Info(ctx, "Order status hasn't changed, skipping update",
			zap.String("status", string(status)))
		return nil
	}

	// Обновляем статус и, если необходимо, обрабатываем возврат средств
	err = c.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Обновляем статус заказа
		err := c.orderStore.UpdateStatus(txCtx, order.ID, status)
		if err != nil {
			return fmt.Errorf("failed to update order status: %w", err)
		}

		logger.Info(txCtx, "Updated order status",
			zap.String("id", order.ID.String()),
			zap.String("old_status", string(order.Status)),
			zap.String("new_status", string(status)))

		// Если заказ провалился, но был покупка, возвращаем средства на счет
		if status == entity.OrderStatusFailed && order.Status != entity.OrderStatusFailed && order.IsBuy() {
			err := c.accountStore.UpdateBalance(txCtx, order.AccountID, order.Amount)
			if err != nil {
				return fmt.Errorf("failed to return funds to account: %w", err)
			}

			logger.Info(txCtx, "Returned funds to account",
				zap.String("account_id", order.AccountID.String()),
				zap.String("amount", order.Amount.String()))
		}

		return nil
	})

	return err
}
