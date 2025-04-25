package finance

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

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

// ParsedDepositRequest contains parsed and validated data from the deposit request.
type parsedDepositRequest struct {
	accountID uuid.UUID
	amount    decimal.Decimal
	currency  string
}

// DepositFundsHandler handles the deposit funds operations
type DepositFundsHandler struct {
	depositStore              store.DepositStore
	accountStore              store.AccountStore
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient
	logger                    *zap.Logger
}

// NewDepositFundsHandler creates a new instance of DepositFundsHandler
func NewDepositFundsHandler(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient,
	logger *zap.Logger,
) *DepositFundsHandler {
	return &DepositFundsHandler{
		depositStore:              depositStore,
		accountStore:              accountStore,
		partnerProxyFinanceClient: partnerProxyFinanceClient,
		logger:                    logger,
	}
}

// Handle processes the deposit funds request and returns the response
// It validates the input, checks the account status, creates a deposit,
// communicates with the partner service, and updates the account balance if needed.
func (h *DepositFundsHandler) Handle(ctx context.Context, req *tradingv1.DepositFundsRequest) (*tradingv1.DepositFundsResponse, error) {
	h.logger.Debug("Handling deposit funds request", zap.Any("request", req))

	parsedReq, err := h.parseAndValidateRequest(req)
	if err != nil {
		h.logger.Error("Failed to validate deposit request", zap.Error(err))
		return nil, err // Status error is already formatted in parseAndValidateRequest
	}

	// Get or create the deposit
	deposit, err := h.getOrCreateDeposit(ctx, req.IdempotencyKey, parsedReq)
	if err != nil {
		h.logger.Error("Failed to get or create deposit", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get or create deposit: %v", err)
	}

	// If deposit is already in a terminal status (completed or failed), return its status
	if deposit.IsTerminated() {
		h.logger.Info("Deposit already in terminal status",
			zap.String("status", string(deposit.Status)),
			zap.String("depositID", deposit.ID.String()))
		return &tradingv1.DepositFundsResponse{
			AccountId: req.AccountId,
			Status:    mapDepositStatusToProto(deposit.Status),
		}, nil
	}

	// Check account existence and status
	account, err := h.getAndValidateAccount(ctx, parsedReq.accountID, parsedReq.currency)
	if err != nil {
		h.logger.Error("Account validation failed",
			zap.String("accountID", parsedReq.accountID.String()),
			zap.Error(err))
		return nil, err
	}

	// Process deposit with partner service
	response, err := h.partnerProxyFinanceClient.CreateDeposit(
		ctx,
		deposit,
		account.ExtID,
	)

	if err != nil {
		h.logger.Error("Failed to create deposit with partner service",
			zap.String("depositID", deposit.ID.String()),
			zap.Error(err))
		if statusErr, ok := status.FromError(err); ok {
			return nil, statusErr.Err()
		}
		return nil, status.Errorf(codes.Internal, "failed to create deposit with partner service: %v", err)
	}

	// Update deposit with external data
	if err := h.depositStore.UpdateExternalData(ctx, deposit.ID, response.ExtID, response.Status); err != nil {
		h.logger.Error("Failed to update deposit with external data",
			zap.String("depositID", deposit.ID.String()),
			zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to update deposit with external data: %v", err)
	}

	// Update local deposit object to return correct status
	deposit.ExtID = response.ExtID
	deposit.Status = response.Status

	h.logger.Info("Deposit processed successfully",
		zap.String("depositID", deposit.ID.String()),
		zap.String("status", string(deposit.Status)))
	return &tradingv1.DepositFundsResponse{
		AccountId: req.AccountId,
		Status:    mapDepositStatusToProto(deposit.Status),
	}, nil
}

// parseAndValidateRequest validates and parses the deposit funds request.
// Returns a structure with parsed data or an error if validation failed.
func (h *DepositFundsHandler) parseAndValidateRequest(req *tradingv1.DepositFundsRequest) (*parsedDepositRequest, error) {
	// Basic parameter validation
	if req.AccountId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "account ID is required")
	}

	if req.Currency == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}

	if req.Amount == nil || req.Amount.Value == "" {
		return nil, status.Errorf(codes.InvalidArgument, "amount is required")
	}

	if req.IdempotencyKey == "" {
		return nil, status.Errorf(codes.InvalidArgument, "idempotency key is required")
	}

	// Parse account ID
	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account ID: %v", err)
	}

	// Parse amount
	amount, err := decimal.NewFromString(req.Amount.Value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount: %v", err)
	}

	// Additional check for positive amount
	if amount.Sign() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}

	return &parsedDepositRequest{
		accountID: accountID,
		amount:    amount,
		currency:  req.Currency,
	}, nil
}

// getOrCreateDeposit gets an existing deposit by idempotency key or creates a new one if not found.
// Also verifies that the existing deposit (if any) matches the current request parameters.
func (h *DepositFundsHandler) getOrCreateDeposit(
	ctx context.Context,
	idempotencyKey string,
	parsedReq *parsedDepositRequest,
) (*entity.Deposit, error) {
	// Check for existing deposit with this idempotency key
	deposit, err := h.depositStore.GetByIdempotencyKey(ctx, idempotencyKey)
	if err == nil {
		// Deposit found, verify it matches the current request parameters
		if deposit.AccountID != parsedReq.accountID {
			return nil, status.Errorf(codes.PermissionDenied,
				"deposit with this idempotency key already exists with different account ID")
		}
		return deposit, nil
	}

	// If error is not "not found", return error
	if !errors.Is(err, entity.ErrNotFound) {
		return nil, fmt.Errorf("failed to check idempotency key: %w", err)
	}

	// Deposit not found, create a new one
	newDeposit := h.createDepositEntity(parsedReq.accountID, parsedReq.amount, parsedReq.currency, idempotencyKey)
	if err := h.depositStore.Create(ctx, newDeposit); err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}

	h.logger.Info("Created new deposit",
		zap.String("depositID", newDeposit.ID.String()),
		zap.String("accountID", parsedReq.accountID.String()))
	return newDeposit, nil
}

// getAndValidateAccount retrieves an account and validates its status and currency
func (h *DepositFundsHandler) getAndValidateAccount(ctx context.Context, accountID uuid.UUID, currency string) (*entity.Account, error) {
	account, err := h.accountStore.GetByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get account: %v", err)
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
func (h *DepositFundsHandler) createDepositEntity(accountID uuid.UUID, amount decimal.Decimal, currency string, idempotencyKey string) *entity.Deposit {
	now := time.Now()
	return &entity.Deposit{
		ID:             uuid.New(),
		AccountID:      accountID,
		Amount:         amount,
		Currency:       currency,
		Status:         entity.DepositStatusPending,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
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
