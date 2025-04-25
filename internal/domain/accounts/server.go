package accounts

import (
	"context"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/store"
)

type Server struct {
	getAccountHandler          *getAccountHandler
	getAccountsByUserIDHandler *getAccountsByUserIDHandler

	tradingv1.UnimplementedAccountServiceServer
}

func NewServer(
	accountStore store.AccountStore,
) *Server {
	return &Server{
		getAccountHandler:          newGetAccountHandler(accountStore),
		getAccountsByUserIDHandler: newGetAccountsByUserIDHandler(accountStore),
	}
}

func (s *Server) GetAccount(ctx context.Context, req *tradingv1.GetAccountRequest) (*tradingv1.GetAccountResponse, error) {
	return s.getAccountHandler.handle(ctx, req)
}

func (s *Server) GetAccountsByUserId(ctx context.Context, req *tradingv1.GetAccountsByUserIdRequest) (*tradingv1.GetAccountsByUserIdResponse, error) {
	return s.getAccountsByUserIDHandler.handle(ctx, req)
}
