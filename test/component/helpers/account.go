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

// TestContext создает контекст для теста с таймаутом
func TestContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	return ctx
}

// GetTestConfig возвращает конфигурацию для тестового окружения
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

// getEnvOrDefault возвращает значение переменной окружения или значение по умолчанию
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// CreateTestAccount создает тестовый аккаунт
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

// CreateAccountWithParams создает тестовый аккаунт с указанными параметрами
func CreateAccountWithParams(ctx context.Context, t *testing.T, accountStore store.AccountStore, accountFixture *entity.Account) *entity.Account {
	err := accountStore.Create(ctx, accountFixture)
	require.NoError(t, err, "Не удалось создать аккаунт")
	return accountFixture
}

// AssertAccountExists проверяет существование аккаунта
func AssertAccountExists(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID) *entity.Account {
	account, err := accountStore.GetByID(ctx, id)
	assert.NoError(t, err, "Аккаунт должен существовать")
	assert.NotNil(t, account, "Аккаунт должен быть найден")
	return account
}

// AssertAccountWithStatus проверяет статус аккаунта
func AssertAccountWithStatus(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, status entity.AccountStatus) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.Equal(t, status, account.Status, "Статус аккаунта должен соответствовать ожидаемому")
	return account
}

// AssertAccountWithBalance проверяет баланс аккаунта
func AssertAccountWithBalance(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, balance decimal.Decimal) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.True(t, account.Balance.Equal(balance), "Баланс аккаунта должен соответствовать ожидаемому")
	return account
}

// AssertAccountWithCurrency проверяет валюту аккаунта
func AssertAccountWithCurrency(ctx context.Context, t *testing.T, accountStore store.AccountStore, id uuid.UUID, currency string) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, id)
	assert.Equal(t, currency, account.Currency, "Валюта аккаунта должна соответствовать ожидаемой")
	return account
}

// AssertUserHasAccounts проверяет, что у пользователя есть определенное количество аккаунтов
func AssertUserHasAccounts(ctx context.Context, t *testing.T, accountStore store.AccountStore, userID string, count int) []*entity.Account {
	accounts, err := accountStore.GetByUserID(ctx, userID)
	assert.NoError(t, err, "Запрос аккаунтов пользователя должен завершиться успешно")
	assert.Len(t, accounts, count, "Количество аккаунтов пользователя должно соответствовать ожидаемому")
	return accounts
}

// AssertAccountWithExtID проверяет внешний ID аккаунта
func AssertAccountWithExtID(ctx context.Context, t *testing.T, accountStore store.AccountStore, accountID uuid.UUID, extID string) *entity.Account {
	account := AssertAccountExists(ctx, t, accountStore, accountID)
	assert.Equal(t, extID, account.ExtID, "Внешний ID аккаунта должен соответствовать ожидаемому")
	return account
}

// GetAccountIDByExtID получает ID аккаунта по внешнему ID
func GetAccountIDByExtID(ctx context.Context, t *testing.T, accountStore store.AccountStore, extID string) uuid.UUID {
	account, err := accountStore.GetByExtID(ctx, extID)
	require.NoError(t, err, "Аккаунт должен быть найден по внешнему ID")
	return account.ID
}
