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

// parsedDepositRequest содержит разобранные и проверенные данные из запроса на пополнение.
type parsedDepositRequest struct {
	accountID uuid.UUID
	amount    decimal.Decimal
}

// depositFundsHandler handles the deposit funds operations
type depositFundsHandler struct {
	depositStore              store.DepositStore
	accountStore              store.AccountStore
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient
}

// newDepositFundsHandler creates a new instance of depositFundsHandler
func newDepositFundsHandler(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	partnerProxyFinanceClient dependency.PartnerProxyFinanceClient,
) *depositFundsHandler {
	return &depositFundsHandler{
		depositStore:              depositStore,
		accountStore:              accountStore,
		partnerProxyFinanceClient: partnerProxyFinanceClient,
	}
}

// Handle processes the deposit funds request and returns the response
// It validates the input, checks the account status, creates a deposit,
// communicates with the partner service, and updates the account balance if needed.
func (h *depositFundsHandler) Handle(ctx context.Context, req *tradingv1.DepositFundsRequest) (*tradingv1.DepositFundsResponse, error) {
	parsedReq, err := h.parseAndValidateRequest(req)
	if err != nil {
		return nil, err // Ошибка уже в формате status.Error
	}

	// Получаем или создаем депозит
	deposit, err := h.getOrCreateDeposit(ctx, req.IdempotencyKey, parsedReq, req.Currency)
	if err != nil {
		// Обрабатываем ошибку получения/создания депозита (уже обернута)
		// Можно добавить логирование или специфическую обработку
		// Возвращаем внутреннюю ошибку, так как это проблема с БД или логикой
		return nil, status.Errorf(codes.Internal, "failed to get or create deposit: %v", err)
	}

	// Если депозит уже в терминальном статусе (завершен или упал), возвращаем его статус
	if deposit.IsTerminated() {
		return &tradingv1.DepositFundsResponse{
			AccountId: req.AccountId,
			Status:    mapDepositStatusToProto(deposit.Status),
		}, nil
	}

	// Check account existence and status
	account, err := h.getAndValidateAccount(ctx, parsedReq.accountID, req.Currency)
	if err != nil {
		return nil, err
	}

	// Process deposit with partner service
	response, err := h.partnerProxyFinanceClient.CreateDeposit(
		ctx,
		deposit,
		account.ExtID,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create deposit with partner service: %w", err)
	}

	// Update deposit with external data
	if err := h.depositStore.UpdateExternalData(ctx, deposit.ID, response.ExtID, response.Status); err != nil {
		return nil, fmt.Errorf("failed to update deposit with external data: %w", err)
	}

	// Обновляем локальный объект deposit для корректного возврата статуса
	deposit.ExtID = response.ExtID
	deposit.Status = response.Status

	return &tradingv1.DepositFundsResponse{
		AccountId: req.AccountId,
		Status:    mapDepositStatusToProto(deposit.Status),
	}, nil
}

// parseAndValidateRequest проверяет и разбирает входной запрос на пополнение.
// Возвращает структуру с разобранными данными или ошибку, если проверка не удалась.
func (h *depositFundsHandler) parseAndValidateRequest(req *tradingv1.DepositFundsRequest) (*parsedDepositRequest, error) {
	// Проверка входных параметров
	if err := h.validateRequest(req); err != nil {
		return nil, err
	}

	accountID, err := uuid.Parse(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "неверный ID аккаунта: %v", err)
	}

	amount, err := decimal.NewFromString(req.Amount.Value)
	if err != nil {
		// Убедимся, что используется правильное поле для значения суммы
		if req.Amount == nil || req.Amount.Value == "" {
			return nil, status.Errorf(codes.InvalidArgument, "сумма обязательна")
		}
		return nil, status.Errorf(codes.InvalidArgument, "неверная сумма: %v", err)
	}
	// Дополнительная проверка, что сумма положительная
	if amount.Sign() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "сумма должна быть положительной")
	}

	return &parsedDepositRequest{
		accountID: accountID,
		amount:    amount,
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

	if req.IdempotencyKey == "" {
		return status.Errorf(codes.InvalidArgument, "idempotency key is required")
	}

	return nil
}

// getOrCreateDeposit получает существующий депозит по ключу идемпотентности
// или создает новый, если он не найден.
func (h *depositFundsHandler) getOrCreateDeposit(
	ctx context.Context,
	idempotencyKey string,
	parsedReq *parsedDepositRequest,
	currency string,
) (*entity.Deposit, error) {
	// Проверяем существование депозита с таким ключом идемпотентности
	deposit, err := h.depositStore.GetByIdempotencyKey(ctx, idempotencyKey)
	if err == nil {
		// Депозит найден, возвращаем его
		return deposit, nil
	}

	// Если ошибка не "не найдено", возвращаем ошибку
	if !errors.Is(err, entity.ErrNotFound) {
		return nil, fmt.Errorf("failed to check idempotency key: %w", err)
	}

	// Депозит не найден, создаем новый
	newDeposit := h.createDepositEntity(parsedReq.accountID, parsedReq.amount, currency, idempotencyKey)
	if err := h.depositStore.Create(ctx, newDeposit); err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}
	// Возвращаем только что созданный депозит
	return newDeposit, nil
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
func (h *depositFundsHandler) createDepositEntity(accountID uuid.UUID, amount decimal.Decimal, currency string, idempotencyKey string) *entity.Deposit {
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
