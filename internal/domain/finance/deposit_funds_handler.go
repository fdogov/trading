package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

// depositFundsHandler handles the deposit funds operations
type depositFundsHandler struct {
	depositStore              store.DepositStore
	accountStore              store.AccountStore
	dbTransactor              store.DBTransactor
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient
}

// newDepositFundsHandler creates a new instance of depositFundsHandler
func newDepositFundsHandler(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient,
) *depositFundsHandler {
	return &depositFundsHandler{
		depositStore:              depositStore,
		accountStore:              accountStore,
		dbTransactor:              dbTransactor,
		partnerProxyFinanceClient: partnerProxyFinanceClient,
	}
}

// Handle processes the deposit funds request and returns the response
// It validates the input, checks the account status, creates a deposit,
// communicates with the partner service, and updates the account balance if needed.
func (h *depositFundsHandler) Handle(ctx context.Context, req *tradingv1.DepositFundsRequest) (*tradingv1.DepositFundsResponse, error) {
	// Validate input parameters
	if err := h.validateRequest(req); err != nil {
		return nil, err
	}

	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account ID: %v", err)
	}

	amount, err := decimal.NewFromString(req.Amount.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount: %v", err)
	}

	var depositStatus entity.DepositStatus
	err = h.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Check account existence and status
		account, err := h.getAndValidateAccount(txCtx, accountID, req.Currency)
		if err != nil {
			return err
		}

		// Create and save deposit entity
		deposit := h.createDepositEntity(accountID, amount, req.Currency)
		if err := h.depositStore.Create(txCtx, deposit); err != nil {
			return fmt.Errorf("failed to create deposit: %w", err)
		}

		// Process deposit with partner service
		extDepositID, status, err := h.processDepositWithPartner(txCtx, account.ExtID, amount, req.Currency)
		if err != nil {
			return err
		}

		// Update deposit with external data
		deposit.ExtID = extDepositID
		deposit.Status = status
		deposit.UpdatedAt = time.Now()
		depositStatus = status

		if err := h.depositStore.Update(txCtx, deposit); err != nil {
			return fmt.Errorf("failed to update deposit with external ID: %w", err)
		}

		// Update account balance if deposit completed successfully
		if status == entity.DepositStatusCompleted {
			if err := h.accountStore.UpdateBalance(txCtx, accountID, amount.String()); err != nil {
				return fmt.Errorf("failed to update account balance: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, h.handleTransactionError(err)
	}

	return &tradingv1.DepositFundsResponse{
		AccountId: req.AccountId,
		Status:    mapDepositStatusToProto(depositStatus),
	}, nil
}

// validateRequest validates the deposit funds request parameters
func (h *depositFundsHandler) validateRequest(req *tradingv1.DepositFundsRequest) error {
	if req.AccountId == "" {
		return status.Errorf(codes.InvalidArgument, "account ID is required")
	}

	if req.Currency == "" {
		return status.Errorf(codes.InvalidArgument, "currency is required")
	}

	if req.Amount == nil || req.Amount.Value == "" {
		return status.Errorf(codes.InvalidArgument, "amount is required")
	}

	return nil
}

// getAndValidateAccount retrieves an account and validates its status and currency
func (h *depositFundsHandler) getAndValidateAccount(ctx context.Context, accountID uuid.UUID, currency string) (*entity.Account, error) {
	account, err := h.accountStore.GetByID(ctx, accountID)
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

	// Verify currency match
	if account.Currency != currency {
		return nil, status.Errorf(codes.InvalidArgument,
			"currency mismatch: account currency %s, deposit currency %s",
			account.Currency, currency)
	}

	return account, nil
}

// createDepositEntity creates a new deposit entity with pending status
func (h *depositFundsHandler) createDepositEntity(accountID uuid.UUID, amount decimal.Decimal, currency string) *entity.Deposit {
	now := time.Now()
	return &entity.Deposit{
		ID:        uuid.New(),
		AccountID: accountID,
		Amount:    amount,
		Currency:  currency,
		Status:    entity.DepositStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// processDepositWithPartner creates a deposit with the partner service
func (h *depositFundsHandler) processDepositWithPartner(
	ctx context.Context,
	accountExtID string,
	amount decimal.Decimal,
	currency string,
) (string, entity.DepositStatus, error) {
	extDepositID, status, err := h.partnerProxyFinanceClient.CreateDeposit(
		ctx,
		accountExtID,
		amount,
		currency,
	)
	if err != nil {
		return "", entity.DepositStatusFailed, fmt.Errorf("failed to create deposit at partner: %w", err)
	}

	return extDepositID, status, nil
}

// handleTransactionError processes errors from the transaction execution
func (h *depositFundsHandler) handleTransactionError(err error) error {
	if st, ok := status.FromError(err); ok {
		return st.Err()
	}
	return status.Errorf(codes.Internal, "failed to deposit funds: %v", err)
}

// mapDepositStatusToProto maps entity deposit status to protobuf deposit status
func mapDepositStatusToProto(status entity.DepositStatus) tradingv1.DepositStatus {
	switch status {
	case entity.DepositStatusPending:
		return tradingv1.DepositStatus_DEPOSIT_STATUS_PENDING
	case entity.DepositStatusCompleted:
		return tradingv1.DepositStatus_DEPOSIT_STATUS_COMPLETED
	case entity.DepositStatusFailed:
		return tradingv1.DepositStatus_DEPOSIT_STATUS_FAILED
	default:
		return tradingv1.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}
