package finance

import (
	"context"
	"go.uber.org/zap"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/store"
)

type Server struct {
	depositFundsHandler *DepositFundsHandler

	tradingv1.UnimplementedFinanceServiceServer
}

func NewServer(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient,
	logger *zap.Logger,
) *Server {
	return &Server{
		depositFundsHandler: NewDepositFundsHandler(depositStore, accountStore, partnerProxyFinanceClient, logger),
	}
}

func (s *Server) DepositFunds(ctx context.Context, req *tradingv1.DepositFundsRequest) (*tradingv1.DepositFundsResponse, error) {
	return s.depositFundsHandler.Handle(ctx, req)
}
