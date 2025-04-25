package finance

import (
	"context"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/store"
)

type Server struct {
	depositFundsHandler *depositFundsHandler

	tradingv1.UnimplementedFinanceServiceServer
}

func NewServer(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient,
) *Server {
	return &Server{
		depositFundsHandler: newDepositFundsHandler(depositStore, accountStore, partnerProxyFinanceClient),
	}
}

func (s *Server) DepositFunds(ctx context.Context, req *tradingv1.DepositFundsRequest) (*tradingv1.DepositFundsResponse, error) {
	return s.depositFundsHandler.Handle(ctx, req)
}
