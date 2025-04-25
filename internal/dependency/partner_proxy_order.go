package dependency

import (
	"context"
	"fmt"
	"time"

	_type "github.com/fdogov/contracts/gen/go/google/type"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"

	partnerproxyv1 "github.com/fdogov/contracts/gen/go/backend/partnerproxy/v1"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/entity"
)

//go:generate mockgen -destination=../mocks/mock_partner_proxy_order_client.go -package=mocks github.com/fdogov/trading/internal/dependency PartnerProxyOrderClient

// PartnerProxyOrderClient представляет интерфейс для взаимодействия с сервисом заказов PartnerProxy
type PartnerProxyOrderClient interface {
	CreateOrder(ctx context.Context, extAccountID string, symbol string, quantity decimal.Decimal, price decimal.Decimal, currency string, side entity.OrderSide) (string, entity.OrderStatus, error)
}

// partnerProxyOrderClient реализует интерфейс PartnerProxyOrderClient
type partnerProxyOrderClient struct {
	client partnerproxyv1.OrderServiceClient
}

// NewPartnerProxyOrderClient создает новый экземпляр PartnerProxyOrderClient
func NewPartnerProxyOrderClient(cfg config.Dependency) (PartnerProxyOrderClient, error) {
	conn, err := NewGrpcConn(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection: %w", err)
	}

	return &partnerProxyOrderClient{
		client: partnerproxyv1.NewOrderServiceClient(conn),
	}, nil
}

// CreateOrder создает заказ через PartnerProxyOrderClient
func (c *partnerProxyOrderClient) CreateOrder(
	ctx context.Context,
	extAccountID string,
	symbol string,
	quantity decimal.Decimal,
	price decimal.Decimal,
	currency string,
	side entity.OrderSide,
) (string, entity.OrderStatus, error) {
	// Подготовка запроса
	var orderSide partnerproxyv1.OrderSide
	switch side {
	case entity.OrderSideBuy:
		orderSide = partnerproxyv1.OrderSide_ORDER_SIDE_BUY
	case entity.OrderSideSell:
		orderSide = partnerproxyv1.OrderSide_ORDER_SIDE_SELL
	default:
		return "", entity.OrderStatusFailed, fmt.Errorf("invalid order side: %s", side)
	}

	// Создаем прото-объекты для decimal
	quantityDecimal := &_type.Decimal{
		Value: quantity.String(),
	}

	priceDecimal := &_type.Decimal{
		Value: price.String(),
	}

	// Отправляем запрос
	resp, err := c.client.CreateOrder(ctx, &partnerproxyv1.CreateOrderRequest{
		IdempotencyKey: uuid.New().String(),
		ExtAccountId:   extAccountID,
		Symbol:         symbol,
		Quantity:       quantityDecimal,
		Price:          priceDecimal,
		CreatedAt:      timestamppb.New(time.Now()),
		Currency:       currency,
		Side:           orderSide,
	})
	if err != nil {
		return "", entity.OrderStatusFailed, fmt.Errorf("failed to create order: %w", err)
	}

	// Определяем статус заказа
	var status entity.OrderStatus
	switch resp.Status {
	case "PENDING":
		status = entity.OrderStatusProcessing
	case "COMPLETED":
		status = entity.OrderStatusCompleted
	case "FAILED":
		status = entity.OrderStatusFailed
	default:
		status = entity.OrderStatusNew
	}

	return resp.OrderId, status, nil
}
