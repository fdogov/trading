package orders

import (
	"context"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/store"
)

type Server struct {
	createOrderHandler *CreateOrderHandler
	getOrderHandler    *getOrderHandler

	tradingv1.UnimplementedOrderServiceServer
}

func NewServer(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
	partnerProxyOrderClient dependency.PartnerProxyOrderClient,
) *Server {
	return &Server{
		createOrderHandler: NewCreateOrderHandler(orderStore, accountStore, dbTransactor, partnerProxyOrderClient),
		getOrderHandler:    newGetOrderHandler(orderStore),
	}
}

func (s *Server) CreateOrder(ctx context.Context, req *tradingv1.CreateOrderRequest) (*tradingv1.CreateOrderResponse, error) {
	return s.createOrderHandler.Handle(ctx, req)
}

func (s *Server) GetOrder(ctx context.Context, req *tradingv1.GetOrderRequest) (*tradingv1.GetOrderResponse, error) {
	return s.getOrderHandler.handle(ctx, req)
}
