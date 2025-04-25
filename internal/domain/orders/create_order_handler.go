package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

type createOrderHandler struct {
	orderStore              store.OrderStore
	accountStore            store.AccountStore
	dbTransactor            store.DBTransactor
	partnerProxyOrderClient dependency.PartnerProxyOrderClient
}

func newCreateOrderHandler(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
	partnerProxyOrderClient dependency.PartnerProxyOrderClient,
) *createOrderHandler {
	return &createOrderHandler{
		orderStore:              orderStore,
		accountStore:            accountStore,
		dbTransactor:            dbTransactor,
		partnerProxyOrderClient: partnerProxyOrderClient,
	}
}

func (h *createOrderHandler) handle(ctx context.Context, req *tradingv1.CreateOrderRequest) (*tradingv1.CreateOrderResponse, error) {
	if req.UserId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "user ID is required")
	}

	if req.AccountId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "account ID is required")
	}

	if req.InstrumentId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "instrument ID is required")
	}

	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account ID: %v", err)
	}

	amount, err := decimal.NewFromString(req.Amount.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount: %v", err)
	}

	quantity, err := decimal.NewFromString(req.Quantity.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid quantity: %v", err)
	}

	var orderType entity.OrderType
	switch req.OrderType {
	case tradingv1.OrderType_ORDER_TYPE_MARKET:
		orderType = entity.OrderTypeMarket
	case tradingv1.OrderType_ORDER_TYPE_LIMIT:
		orderType = entity.OrderTypeLimit
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order type")
	}

	var side entity.OrderSide
	switch req.Side {
	case tradingv1.OrderSide_ORDER_SIDE_BUY:
		side = entity.OrderSideBuy
	case tradingv1.OrderSide_ORDER_SIDE_SELL:
		side = entity.OrderSideSell
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order side")
	}

	var order *entity.Order
	err = h.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Проверяем аккаунт
		account, err := h.accountStore.GetByID(txCtx, accountID)
		if err != nil {
			if errors.Is(err, entity.ErrNotFound) {
				return status.Errorf(codes.NotFound, "account not found")
			}
			return fmt.Errorf("failed to get account: %w", err)
		}

		if !account.IsActive() {
			if account.IsBlocked() {
				return status.Errorf(codes.FailedPrecondition, "account is blocked")
			}
			return status.Errorf(codes.FailedPrecondition, "account is inactive")
		}

		// Для покупки проверяем достаточно ли средств
		if side == entity.OrderSideBuy {
			if !account.HasSufficientFunds(amount) {
				return status.Errorf(codes.FailedPrecondition, "insufficient funds")
			}

			// Резервируем средства (уменьшаем баланс)
			err = h.accountStore.UpdateBalance(txCtx, accountID, amount.Neg().String())
			if err != nil {
				return fmt.Errorf("failed to update account balance: %w", err)
			}
		}

		// Создаем заказ
		now := time.Now()
		order = &entity.Order{
			ID:           uuid.New(),
			UserID:       req.UserId,
			AccountID:    accountID,
			InstrumentID: req.InstrumentId,
			Amount:       amount,
			Quantity:     quantity,
			OrderType:    orderType,
			Side:         side,
			Status:       entity.OrderStatusNew,
			Description:  req.Description,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		// Сохраняем заказ в базе
		err = h.orderStore.Create(txCtx, order)
		if err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// Создаем заказ у партнера
		extOrderID, orderStatus, err := h.partnerProxyOrderClient.CreateOrder(
			txCtx,
			account.ExtID,
			req.InstrumentId,
			quantity,
			amount,
			account.Currency,
			side,
		)
		if err != nil {
			return fmt.Errorf("failed to create order at partner: %w", err)
		}

		// Обновляем заказ с данными от партнера
		order.ExtID = extOrderID
		order.Status = orderStatus
		order.UpdatedAt = time.Now()

		err = h.orderStore.Update(txCtx, order)
		if err != nil {
			return fmt.Errorf("failed to update order with external ID: %w", err)
		}

		return nil
	})

	if err != nil {
		if st, ok := status.FromError(err); ok {
			return nil, st.Err()
		}
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	return &tradingv1.CreateOrderResponse{
		Order: mapEntityOrderToProto(order),
	}, nil
}

func mapEntityOrderToProto(order *entity.Order) *tradingv1.Order {
	var orderType tradingv1.OrderType
	switch order.OrderType {
	case entity.OrderTypeMarket:
		orderType = tradingv1.OrderType_ORDER_TYPE_MARKET
	case entity.OrderTypeLimit:
		orderType = tradingv1.OrderType_ORDER_TYPE_LIMIT
	default:
		orderType = tradingv1.OrderType_ORDER_TYPE_UNSPECIFIED
	}

	var side tradingv1.OrderSide
	switch order.Side {
	case entity.OrderSideBuy:
		side = tradingv1.OrderSide_ORDER_SIDE_BUY
	case entity.OrderSideSell:
		side = tradingv1.OrderSide_ORDER_SIDE_SELL
	default:
		side = tradingv1.OrderSide_ORDER_SIDE_UNSPECIFIED
	}

	var status tradingv1.OrderStatus
	switch order.Status {
	case entity.OrderStatusNew:
		status = tradingv1.OrderStatus_ORDER_STATUS_NEW
	case entity.OrderStatusProcessing:
		status = tradingv1.OrderStatus_ORDER_STATUS_PROCESSING
	case entity.OrderStatusCompleted:
		status = tradingv1.OrderStatus_ORDER_STATUS_COMPLETED
	case entity.OrderStatusCancelled:
		status = tradingv1.OrderStatus_ORDER_STATUS_CANCELLED
	case entity.OrderStatusFailed:
		status = tradingv1.OrderStatus_ORDER_STATUS_FAILED
	default:
		status = tradingv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}

	return &tradingv1.Order{
		Id:           order.ID.String(),
		UserId:       order.UserID,
		AccountId:    order.AccountID.String(),
		InstrumentId: order.InstrumentID,
		Amount:       order.Amount.String(),
		Quantity:     order.Quantity.String(),
		OrderType:    orderType,
		Side:         side,
		Status:       status,
		Description:  order.Description,
		CreatedAt:    timestamppb.New(order.CreatedAt),
		UpdatedAt:    timestamppb.New(order.UpdatedAt),
	}
}
