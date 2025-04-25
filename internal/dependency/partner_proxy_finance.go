package dependency

import (
	"context"
	"fmt"
	"time"

	_type "github.com/fdogov/contracts/gen/go/google/type"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"

	partnerproxyv1 "github.com/fdogov/contracts/gen/go/backend/partnerproxy/v1"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/entity"
)

//go:generate mockgen -destination=../mocks/mock_partner_proxy_finance_client.go -package=mocks github.com/fdogov/trading/internal/dependency PartnerProxyFinanceClient

// PartnerProxyFinanceClient представляет интерфейс для взаимодействия с финансовым сервисом PartnerProxy
type PartnerProxyFinanceClient interface {
	CreateDeposit(ctx context.Context, extAccountID string, amount decimal.Decimal, currency string) (string, entity.DepositStatus, error)
}

// partnerProxyFinanceClient реализует интерфейс PartnerProxyFinanceClient
type partnerProxyFinanceClient struct {
	client partnerproxyv1.FinanceServiceClient
}

// NewPartnerProxyFinanceClient создает новый экземпляр PartnerProxyFinanceClient
func NewPartnerProxyFinanceClient(cfg config.Dependency) (PartnerProxyFinanceClient, error) {
	conn, err := NewGrpcConn(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection: %w", err)
	}

	return &partnerProxyFinanceClient{
		client: partnerproxyv1.NewFinanceServiceClient(conn),
	}, nil
}

// CreateDeposit создает депозит через PartnerProxyFinanceClient
func (c *partnerProxyFinanceClient) CreateDeposit(
	ctx context.Context,
	extAccountID string,
	amount decimal.Decimal,
	currency string,
) (string, entity.DepositStatus, error) {
	// Создаем прото-объект для decimal
	amountDecimal := &_type.Decimal{
		Value: amount.String(),
	}

	// Отправляем запрос
	resp, err := c.client.CreateDeposit(ctx, &partnerproxyv1.CreateDepositRequest{
		IdempotencyKey: uuid.New().String(),
		ExtAccountId:   extAccountID,
		Amount:         amountDecimal,
		Currency:       currency,
		CreatedAt:      timestamppb.New(time.Now()),
	})
	if err != nil {
		return "", entity.DepositStatusFailed, fmt.Errorf("failed to create deposit: %w", err)
	}

	// Определяем статус депозита
	var status entity.DepositStatus
	switch resp.Status {
	case partnerproxyv1.DepositStatus_DEPOSIT_STATUS_PENDING:
		status = entity.DepositStatusPending
	case partnerproxyv1.DepositStatus_DEPOSIT_STATUS_COMPLETED:
		status = entity.DepositStatusCompleted
	case partnerproxyv1.DepositStatus_DEPOSIT_STATUS_FAILED:
		status = entity.DepositStatusFailed
	default:
		status = entity.DepositStatusPending
	}

	return resp.DepositId, status, nil
}
