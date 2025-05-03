package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store"
)

// OrderRequest contains parsed and validated data from the create order request.
type OrderRequest struct {
	UserID         string
	AccountID      uuid.UUID
	InstrumentID   string
	Amount         decimal.Decimal
	Quantity       decimal.Decimal
	OrderType      entity.OrderType
	Side           entity.OrderSide
	Description    string
	IdempotencyKey string
}

// CreateOrderHandler handles the creation of new orders
type CreateOrderHandler struct {
	orderStore              store.OrderStore
	accountStore            store.AccountStore
	dbTransactor            store.DBTransactor
	partnerProxyOrderClient dependency.PartnerProxyOrderClient
}

// NewCreateOrderHandler creates a new instance of CreateOrderHandler
func NewCreateOrderHandler(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
	partnerProxyOrderClient dependency.PartnerProxyOrderClient,
) *CreateOrderHandler {
	return &CreateOrderHandler{
		orderStore:              orderStore,
		accountStore:            accountStore,
		dbTransactor:            dbTransactor,
		partnerProxyOrderClient: partnerProxyOrderClient,
	}
}

// Handle processes the create order request and returns the response
// It validates the input, checks for idempotency, validates the account,
// creates the order, communicates with the partner service, and updates the status.
func (h *CreateOrderHandler) Handle(ctx context.Context, req *tradingv1.CreateOrderRequest) (*tradingv1.CreateOrderResponse, error) {
	logger.Debug(ctx, "Handling create order request", zap.Any("request", req))

	// Parse and validate the request
	orderReq, err := h.validateRequest(req)
	if err != nil {
		logger.Error(ctx, "Failed to validate order request", zap.Error(err))
		return nil, err // Status error is already formatted in validateRequest
	}

	// Get existing order or create a new one
	order, err := h.getOrCreateOrder(ctx, orderReq)
	if err != nil {
		logger.Error(ctx, "Failed to get or create order", zap.Error(err))
		return nil, err
	}

	// If the order is in a terminal state, return it without further processing
	if order.IsTerminal() {
		logger.Info(ctx, "Order is already in terminal state, returning it",
			zap.String("order_id", order.ID.String()),
			zap.String("status", string(order.Status)))
		return &tradingv1.CreateOrderResponse{
			Order: mapEntityOrderToProto(order),
		}, nil
	}

	// For non-terminal states or new orders, continue with partner integration
	return h.continueOrderProcessing(ctx, order, orderReq)
}

// getOrCreateOrder tries to get an order by idempotency key or creates a new one if not found
func (h *CreateOrderHandler) getOrCreateOrder(ctx context.Context, orderReq *OrderRequest) (*entity.Order, error) {
	// Try to get an existing order by idempotency key
	existingOrder, err := h.orderStore.GetByIdempotencyKey(ctx, orderReq.IdempotencyKey)
	if err == nil {
		// Order with this idempotency key already exists
		logger.Info(ctx, "Order with idempotency key already exists",
			zap.String("idempotency_key", orderReq.IdempotencyKey),
			zap.String("order_id", existingOrder.ID.String()),
			zap.String("status", string(existingOrder.Status)))
		return existingOrder, nil
	}

	// If error is not "not found", there's a problem with the database
	if !errors.Is(err, entity.ErrNotFound) {
		logger.Error(ctx, "Failed to check for existing order",
			zap.String("idempotency_key", orderReq.IdempotencyKey),
			zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to check for existing order: %v", err)
	}

	// Create the order with NEW status
	order, err := h.createOrder(ctx, orderReq)
	if err != nil {
		logger.Error(ctx, "Failed to create order", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	return order, nil
}

// continueOrderProcessing continues processing an existing or newly created order
// by communicating with the partner service and updating the order status
func (h *CreateOrderHandler) continueOrderProcessing(ctx context.Context, order *entity.Order, orderReq *OrderRequest) (*tradingv1.CreateOrderResponse, error) {
	if order.IsProcessing() {
		logger.Info(ctx, "Order is already being processed, skipping partner call",
			zap.String("order_id", order.ID.String()),
			zap.String("status", string(order.Status)))
		return &tradingv1.CreateOrderResponse{
			Order: mapEntityOrderToProto(order),
		}, nil
	}

	// Order doesn't exist, create a new one - first validate the account
	account, err := h.validateAccount(ctx, orderReq)
	if err != nil {
		logger.Error(ctx, "Account validation failed", zap.Error(err))
		return nil, err
	}

	// Create order at the partner
	extOrderID, orderStatus, err := h.partnerProxyOrderClient.CreateOrder(
		ctx,
		account.ExtID,
		order.InstrumentID,
		order.Quantity,
		order.Amount,
		account.Currency,
		order.Side,
	)
	if err != nil {
		logger.Error(ctx, "Failed to create order at partner",
			zap.String("order_id", order.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create order at partner: %w", err)
	}

	// Update order with partner data
	order.ExtID = extOrderID
	order.Status = orderStatus
	order.UpdatedAt = time.Now()

	err = h.orderStore.Update(ctx, order)
	if err != nil {
		logger.Error(ctx, "Failed to update order with external ID",
			zap.String("order_id", order.ID.String()),
			zap.String("ext_order_id", extOrderID),
			zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to update order with external ID: %v", err)
	}

	logger.Info(ctx, "Order processed with partner",
		zap.String("order_id", order.ID.String()),
		zap.String("ext_order_id", extOrderID),
		zap.String("status", string(orderStatus)))

	return &tradingv1.CreateOrderResponse{
		Order: mapEntityOrderToProto(order),
	}, nil
}

// validateRequest validates and parses the create order request.
// Returns a structure with parsed data or an error if validation failed.
func (h *CreateOrderHandler) validateRequest(req *tradingv1.CreateOrderRequest) (*OrderRequest, error) {
	// Basic parameter validation
	if req.UserId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "user ID is required")
	}

	if req.AccountId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "account ID is required")
	}

	if req.InstrumentId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "instrument ID is required")
	}

	if req.Amount == nil || req.Amount.Value == "" {
		return nil, status.Errorf(codes.InvalidArgument, "amount is required")
	}

	if req.Quantity == nil || req.Quantity.Value == "" {
		return nil, status.Errorf(codes.InvalidArgument, "quantity is required")
	}

	if req.IdempotencyKey == "" {
		return nil, status.Errorf(codes.InvalidArgument, "idempotency key is required")
	}

	// Parse account ID
	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account ID format: %v", err)
	}

	// Parse amount
	amount, err := decimal.NewFromString(req.Amount.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount format: %v", err)
	}

	// Parse quantity
	quantity, err := decimal.NewFromString(req.Quantity.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid quantity format: %v", err)
	}

	// Additional validations
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}

	if quantity.LessThanOrEqual(decimal.Zero) {
		return nil, status.Errorf(codes.InvalidArgument, "quantity must be positive")
	}

	// Validate order type
	var orderType entity.OrderType
	switch req.OrderType {
	case tradingv1.OrderType_ORDER_TYPE_MARKET:
		orderType = entity.OrderTypeMarket
	case tradingv1.OrderType_ORDER_TYPE_LIMIT:
		orderType = entity.OrderTypeLimit
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order type")
	}

	// Validate order side
	var side entity.OrderSide
	switch req.Side {
	case tradingv1.OrderSide_ORDER_SIDE_BUY:
		side = entity.OrderSideBuy
	case tradingv1.OrderSide_ORDER_SIDE_SELL:
		side = entity.OrderSideSell
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order side")
	}

	return &OrderRequest{
		UserID:         req.UserId,
		AccountID:      accountID,
		InstrumentID:   req.InstrumentId,
		Amount:         amount,
		Quantity:       quantity,
		OrderType:      orderType,
		Side:           side,
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
	}, nil
}

// validateAccount retrieves an account and validates its status and funds
func (h *CreateOrderHandler) validateAccount(ctx context.Context, orderReq *OrderRequest) (*entity.Account, error) {
	account, err := h.accountStore.GetByID(ctx, orderReq.AccountID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found")
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Verify account status
	if !account.IsActive() {
		if account.IsBlocked() {
			return nil, status.Errorf(codes.FailedPrecondition, "account is blocked")
		}
		return nil, status.Errorf(codes.FailedPrecondition, "account is inactive")
	}

	// For buy orders, check if there are sufficient funds
	if orderReq.Side == entity.OrderSideBuy {
		if !account.HasSufficientFunds(orderReq.Amount) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds")
		}
	}

	return account, nil
}

// createOrder creates a new order entity in the database
func (h *CreateOrderHandler) createOrder(ctx context.Context, orderReq *OrderRequest) (*entity.Order, error) {
	// Create the order entity
	now := time.Now()
	order := &entity.Order{
		ID:             uuid.New(),
		UserID:         orderReq.UserID,
		AccountID:      orderReq.AccountID,
		InstrumentID:   orderReq.InstrumentID,
		Amount:         orderReq.Amount,
		Quantity:       orderReq.Quantity,
		OrderType:      orderReq.OrderType,
		Side:           orderReq.Side,
		Status:         entity.OrderStatusNew,
		Description:    orderReq.Description,
		IdempotencyKey: orderReq.IdempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Save the order in database
	err := h.orderStore.Create(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("failed to create order in database: %w", err)
	}

	logger.Info(ctx, "Created new order",
		zap.String("order_id", order.ID.String()),
		zap.String("user_id", orderReq.UserID),
		zap.String("account_id", orderReq.AccountID.String()),
		zap.String("idempotency_key", orderReq.IdempotencyKey))

	return order, nil
}

// mapEntityOrderToProto maps entity order model to protobuf response model
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
