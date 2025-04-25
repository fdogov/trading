package dependency

import (
	"context"
	"fmt"

	_type "github.com/fdogov/contracts/gen/go/google/type"
	"google.golang.org/protobuf/types/known/timestamppb"

	partnerproxyv1 "github.com/fdogov/contracts/gen/go/backend/partnerproxy/v1"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/entity"
)

//go:generate moq -rm -out gen/partner_proxy_finance_client_mock.go -pkg dependencygen -fmt goimports . PartnerProxyFinanceClient

// PartnerProxyFinanceClient represents an interface for interacting with the PartnerProxy finance service
type PartnerProxyFinanceClient interface {
	CreateDeposit(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*DepositResponse, error)
}

// partnerProxyFinanceClient implements the PartnerProxyFinanceClient interface
type partnerProxyFinanceClient struct {
	client partnerproxyv1.FinanceServiceClient
}

// NewPartnerProxyFinanceClient creates a new instance of PartnerProxyFinanceClient
func NewPartnerProxyFinanceClient(cfg config.Dependency) (PartnerProxyFinanceClient, error) {
	conn, err := NewGrpcConn(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection: %w", err)
	}

	return &partnerProxyFinanceClient{
		client: partnerproxyv1.NewFinanceServiceClient(conn),
	}, nil
}

// CreateDeposit creates a deposit through PartnerProxyFinanceClient
func (c *partnerProxyFinanceClient) CreateDeposit(
	ctx context.Context,
	deposit *entity.Deposit,
	extAccountID string,
) (*DepositResponse, error) {
	// Create a proto object for decimal
	amountDecimal := &_type.Decimal{
		Value: deposit.Amount.String(),
	}

	// Send the request
	resp, err := c.client.CreateDeposit(ctx, &partnerproxyv1.CreateDepositRequest{
		IdempotencyKey: deposit.ID.String(),
		ExtAccountId:   extAccountID,
		Amount:         amountDecimal,
		Currency:       deposit.Currency,
		CreatedAt:      timestamppb.New(deposit.CreatedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}

	// Determine deposit status
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

	return &DepositResponse{
		ExtID:  resp.DepositId,
		Status: status,
	}, nil
}

// DepositResponse represents a response from the partner service for a deposit
type DepositResponse struct {
	ExtID  string
	Status entity.DepositStatus
}
