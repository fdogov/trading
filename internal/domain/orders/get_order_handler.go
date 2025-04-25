package orders

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

type getOrderHandler struct {
	orderStore store.OrderStore
}

func newGetOrderHandler(orderStore store.OrderStore) *getOrderHandler {
	return &getOrderHandler{
		orderStore: orderStore,
	}
}

func (h *getOrderHandler) handle(ctx context.Context, req *tradingv1.GetOrderRequest) (*tradingv1.GetOrderResponse, error) {
	if req.OrderId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order ID is required")
	}

	orderID, err := uuid.Parse(req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid order ID: %v", err)
	}

	order, err := h.orderStore.GetByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get order: %v", err)
	}

	return &tradingv1.GetOrderResponse{
		Order: mapEntityOrderToProto(order),
	}, nil
}
