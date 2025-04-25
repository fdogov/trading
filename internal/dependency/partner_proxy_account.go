package dependency

import (
	"context"
	"fmt"

	partnerproxyv1 "github.com/fdogov/contracts/gen/go/backend/partnerproxy/v1"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/config"
)

//go:generate moq -rm -out gen/partner_proxy_account_client_mock.go -pkg dependencygen -fmt goimports . PartnerProxyAccountClient

// PartnerProxyAccountClient represents an interface for interacting with the PartnerProxy account service
type PartnerProxyAccountClient interface {
	GetAccountBalance(ctx context.Context, extAccountID string) (*decimal.Decimal, error)
}

// partnerProxyAccountClient implements the PartnerProxyAccountClient interface
type partnerProxyAccountClient struct {
	client partnerproxyv1.AccountServiceClient
}

// NewPartnerProxyAccountClient creates a new instance of PartnerProxyAccountClient
func NewPartnerProxyAccountClient(cfg config.Dependency) (PartnerProxyAccountClient, error) {
	conn, err := NewGrpcConn(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection: %w", err)
	}

	return &partnerProxyAccountClient{
		client: partnerproxyv1.NewAccountServiceClient(conn),
	}, nil
}

// GetAccountBalance retrieves the account balance for a given external account ID
func (c *partnerProxyAccountClient) GetAccountBalance(
	ctx context.Context,
	extAccountID string,
) (*decimal.Decimal, error) {
	// Send the request
	resp, err := c.client.GetAccount(ctx, &partnerproxyv1.GetAccountRequest{
		ExtAccountId: extAccountID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Convert the proto response to an entity
	balance, err := c.getBalance(resp.Account)
	if err != nil {
		return nil, fmt.Errorf("failed to map account from proto: %w", err)
	}

	return balance, nil
}

// mapAccountFromProto converts proto account to entity account
func (c *partnerProxyAccountClient) getBalance(protoAccount *partnerproxyv1.Account) (*decimal.Decimal, error) {
	if protoAccount == nil {
		return nil, fmt.Errorf("proto account is nil")
	}

	// Parse the balance decimal
	balance, err := decimal.NewFromString(protoAccount.Balance.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse account balance: %w", err)
	}
	return &balance, nil
}
