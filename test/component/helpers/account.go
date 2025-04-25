package helpers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

// TestContext creates a context for test with timeout
func TestContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	return ctx
}

// GetTestConfig returns configuration for test environment
func GetTestConfig() map[string]string {
	return map[string]string{
		"TEST_DB_HOST":     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		"TEST_DB_PORT":     getEnvOrDefault("TEST_DB_PORT", "5432"),
		"TEST_DB_USER":     getEnvOrDefault("TEST_DB_USER", "trading"),
		"TEST_DB_PASSWORD": getEnvOrDefault("TEST_DB_PASSWORD", "trading"),
		"TEST_DB_NAME":     getEnvOrDefault("TEST_DB_NAME", "test"),
		"TEST_DB_SSLMODE":  getEnvOrDefault("TEST_DB_SSLMODE", "disable"),
	}
}

// getEnvOrDefault returns environment variable value or default value
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// CreateTestAccount creates a test account
func CreateTestAccount(ctx context.Context, t *testing.T, accountStore store.AccountStore, userID string) *entity.Account {
	account := &entity.Account{
		ID:        uuid.New(),
		UserID:    userID,
		ExtID:     uuid.NewString(),
		Balance:   decimal.NewFromInt(0),
		Currency:  "USD",
		Status:    entity.AccountStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := accountStore.Create(ctx, account)
	require.NoError(t, err)
	return account
}

// CreateAccountWithParams creates a test account with specified parameters
func CreateAccountWithParams(ctx context.Context, t *testing.T, accountStore store.AccountStore, accountFixture *entity.Account) *entity.Account {
	err := accountStore.Create(ctx, accountFixture)
	require.NoError(t, err, "Failed to create account")
	return accountFixture
}

// AssertAccountExists checks if account exists
func AssertAccountExists(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID) *entity.Account {
	account, err := accountStore.GetByID(ctx, id)
	assert.NoError(t, err, "Account should exist")
	assert.NotNil(t, account, "Account should be found")
	return account
}

// AssertAccountWithStatus checks account status
func AssertAccountWithStatus(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, status entity.AccountStatus) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.Equal(t, status, account.Status, "Account status should match expected")
	return account
}

// AssertAccountWithBalance checks account balance
func AssertAccountWithBalance(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, balance decimal.Decimal) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.True(t, account.Balance.Equal(balance), "Account balance should match expected")
	return account
}

// AssertAccountWithCurrency checks account currency
func AssertAccountWithCurrency(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, currency string) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.Equal(t, currency, account.Currency, "Account currency should match expected")
	return account
}

// AssertUserHasAccounts checks that user has specific number of accounts
func AssertUserHasAccounts(ctx context.Context, t *testing.T, accountStore store.AccountStore, userID string, count int) []*entity.Account {
	accounts, err := accountStore.GetByUserID(ctx, userID)
	assert.NoError(t, err, "User accounts query should complete successfully")
	assert.Len(t, accounts, count, "Number of user accounts should match expected")
	return accounts
}

// AssertAccountWithExtID checks account external ID
func AssertAccountWithExtID(ctx context.Context, t *testing.T, accountStore store.AccountStore, accountID uuid.UUID, extID string) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, accountID)
	assert.Equal(t, extID, account.ExtID, "Account external ID should match expected")
	return account
}

// GetAccountIDByExtID gets account ID by external ID
func GetAccountIDByExtID(ctx context.Context, t *testing.T, accountStore store.AccountStore, extID string) uuid.UUID {
	account, err := accountStore.GetByExtID(ctx, extID)
	require.NoError(t, err, "Account should be found by external ID")
	return account.ID
}
