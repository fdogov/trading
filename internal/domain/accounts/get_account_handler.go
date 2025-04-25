package accounts

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

type getAccountHandler struct {
	accountStore store.AccountStore
}

func newGetAccountHandler(accountStore store.AccountStore) *getAccountHandler {
	return &getAccountHandler{
		accountStore: accountStore,
	}
}

func (h *getAccountHandler) handle(ctx context.Context, req *tradingv1.GetAccountRequest) (*tradingv1.GetAccountResponse, error) {
	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account ID: %v", err)
	}

	account, err := h.accountStore.GetByID(ctx, accountID)
	if err == nil {
		return &tradingv1.GetAccountResponse{
			Account: mapEntityAccountToProto(account),
		}, nil
	}
	if errors.Is(err, entity.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}
	return nil, status.Errorf(codes.Internal, "failed to get account: %v", err)
}

func mapEntityAccountToProto(account *entity.Account) *tradingv1.Account {
	var status tradingv1.AccountStatus
	switch account.Status {
	case entity.AccountStatusActive:
		status = tradingv1.AccountStatus_ACCOUNT_STATUS_ACTIVE
	case entity.AccountStatusInactive:
		status = tradingv1.AccountStatus_ACCOUNT_STATUS_INACTIVE
	case entity.AccountStatusBlocked:
		status = tradingv1.AccountStatus_ACCOUNT_STATUS_BLOCKED
	default:
		status = tradingv1.AccountStatus_ACCOUNT_STATUS_UNSPECIFIED
	}

	return &tradingv1.Account{
		Id:        account.ID.String(),
		UserId:    account.UserID,
		Balance:   account.Balance.String(),
		Currency:  account.Currency,
		Status:    status,
		CreatedAt: timestamppb.New(account.CreatedAt),
		UpdatedAt: timestamppb.New(account.UpdatedAt),
	}
}
