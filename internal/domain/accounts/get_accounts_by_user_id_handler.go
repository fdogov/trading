package accounts

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/store"
)

type getAccountsByUserIDHandler struct {
	accountStore store.AccountStore
}

func newGetAccountsByUserIDHandler(accountStore store.AccountStore) *getAccountsByUserIDHandler {
	return &getAccountsByUserIDHandler{
		accountStore: accountStore,
	}
}

func (h *getAccountsByUserIDHandler) handle(ctx context.Context, req *tradingv1.GetAccountsByUserIdRequest) (*tradingv1.GetAccountsByUserIdResponse, error) {
	if req.UserId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "user ID is required")
	}

	accounts, err := h.accountStore.GetByUserID(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get accounts: %v", err)
	}

	protoAccounts := make([]*tradingv1.Account, 0, len(accounts))
	for _, account := range accounts {
		protoAccounts = append(protoAccounts, mapEntityAccountToProto(account))
	}

	return &tradingv1.GetAccountsByUserIdResponse{
		Accounts: protoAccounts,
	}, nil
}
