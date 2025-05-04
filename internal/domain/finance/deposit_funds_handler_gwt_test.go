package finance

import (
	"context"
	"errors"
	"github.com/fdogov/trading/internal/dependency"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dependencygen "github.com/fdogov/trading/internal/dependency/gen"
	"github.com/fdogov/trading/internal/entity"
	storemock "github.com/fdogov/trading/internal/store/gen"
)

// TestData содержит все данные и состояния для каждого теста
type TestData struct {
	// Тестовый контекст
	ctx context.Context
	t   *testing.T

	// Компонент, который тестируем
	handler *DepositFundsHandler

	// Моки зависимостей
	depositStore  *storemock.DepositStoreMock
	accountStore  *storemock.AccountStoreMock
	partnerClient *dependencygen.PartnerProxyFinanceClientMock

	// Тестовые данные
	accountID       uuid.UUID
	extAccountID    string
	amount          decimal.Decimal
	currency        string
	idempotencyKey  string
	existingDeposit *entity.Deposit
	existingAccount *entity.Account

	// Ошибка
	err error
}

// TestCase определяет тестовый сценарий в формате GWT
type TestCase struct {
	name  string
	given func(td *TestData)            // Given: начальное состояние и запрос
	when  func(td *TestData)            // When: выполнение действия (обработка запроса)
	then  func(td *TestData, err error) // Then: проверка результатов
}

// createTestData создает тестовые данные для каждого теста
func createTestData(t *testing.T) *TestData {
	// Инициализация контекста
	ctx := context.Background()

	// Генерация тестовых данных
	accountID := uuid.New()
	extAccountID := "ext-account-id"
	amount := decimal.NewFromInt(100)
	currency := "USD"
	idempotencyKey := uuid.NewString()

	// Создание моков
	depositStore := &storemock.DepositStoreMock{}
	accountStore := &storemock.AccountStoreMock{}
	partnerClient := &dependencygen.PartnerProxyFinanceClientMock{}

	// Создание обработчика для тестирования
	handler := NewDepositFundsHandler(depositStore, accountStore, partnerClient)

	return &TestData{
		ctx:             ctx,
		t:               t,
		handler:         handler,
		depositStore:    depositStore,
		accountStore:    accountStore,
		partnerClient:   partnerClient,
		accountID:       accountID,
		extAccountID:    extAccountID,
		amount:          amount,
		currency:        currency,
		idempotencyKey:  idempotencyKey,
		existingDeposit: nil,
		existingAccount: nil,
		err:             nil,
	}
}

// TestDepositFundsHandler_getOrCreateDeposit проверяет функцию getOrCreateDeposit
func TestDepositFundsHandler_getOrCreateDeposit(t *testing.T) {
	// Определение тестовых сценариев
	testCases := []TestCase{
		// 1. Создание нового депозита
		{
			name: "Create new deposit",
			given: func(td *TestData) {
				// Given: подготовлены данные для создания нового депозита
				td.idempotencyKey = "new-key"
			},
			when: func(td *TestData) {
				// When: настраиваем моки и вызываем функцию getOrCreateDeposit

				// Настройка моков
				td.depositStore.GetByIdempotencyKeyFunc = func(ctx context.Context, key string) (*entity.Deposit, error) {
					return nil, entity.ErrNotFound
				}

				td.depositStore.CreateFunc = func(ctx context.Context, deposit *entity.Deposit) error {
					return nil
				}

				// Подготовка запроса
				req := &DepositRequest{
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
				}

				// Вызов тестируемой функции
				deposit, err := td.handler.getOrCreateDeposit(td.ctx, td.idempotencyKey, req)
				td.existingDeposit = deposit
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.NoError(td.t, err)
				require.NotNil(td.t, td.existingDeposit)

				assert.Equal(td.t, td.accountID, td.existingDeposit.AccountID)
				assert.True(td.t, td.amount.Equal(td.existingDeposit.Amount))
				assert.Equal(td.t, td.currency, td.existingDeposit.Currency)

				assert.Equal(td.t, 1, len(td.depositStore.CreateCalls()))
			},
		},

		// 2. Получение существующего депозита
		{
			name: "Return existing deposit",
			given: func(td *TestData) {
				// Given: депозит с таким ключом идемпотентности уже существует

				// Используем фиксированный ID аккаунта
				td.accountID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
				now := time.Now()

				td.depositStore.GetByIdempotencyKeyFunc = func(ctx context.Context, key string) (*entity.Deposit, error) {
					return &entity.Deposit{
						ID:             uuid.New(),
						AccountID:      td.accountID, // Используем тот же ID аккаунта, что и в запросе
						Amount:         td.amount,
						Currency:       td.currency,
						Status:         entity.DepositStatusPending,
						IdempotencyKey: key,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil
				}

				td.idempotencyKey = "existing-key"
			},
			when: func(td *TestData) {
				// When: вызов функции getOrCreateDeposit
				req := &DepositRequest{
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
				}

				deposit, err := td.handler.getOrCreateDeposit(td.ctx, td.idempotencyKey, req)
				td.existingDeposit = deposit
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.NoError(td.t, err)
				require.NotNil(td.t, td.existingDeposit)

				assert.Equal(td.t, td.accountID, td.existingDeposit.AccountID)
				assert.True(td.t, td.amount.Equal(td.existingDeposit.Amount))
				assert.Equal(td.t, td.currency, td.existingDeposit.Currency)

				// Проверка что новый депозит не создавался
				assert.Empty(td.t, td.depositStore.CreateCalls())
			},
		},

		// 3. Конфликт ключа идемпотентности
		{
			name: "Idempotency key exists for different account",
			given: func(td *TestData) {
				// Given: депозит с таким ключом идемпотентности существует для другого аккаунта

				td.depositStore.GetByIdempotencyKeyFunc = func(ctx context.Context, key string) (*entity.Deposit, error) {
					return &entity.Deposit{
						ID:             uuid.New(),
						AccountID:      uuid.New(), // Другой ID аккаунта чем в запросе
						Amount:         td.amount,
						Currency:       td.currency,
						Status:         entity.DepositStatusPending,
						IdempotencyKey: key,
					}, nil
				}

				td.idempotencyKey = "conflict-key"
			},
			when: func(td *TestData) {
				// When: вызов функции getOrCreateDeposit
				req := &DepositRequest{
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
				}

				deposit, err := td.handler.getOrCreateDeposit(td.ctx, td.idempotencyKey, req)
				td.existingDeposit = deposit
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "deposit with this idempotency key already exists for a different account")
				assert.Nil(td.t, td.existingDeposit)
			},
		},

		// 4. Ошибка базы данных при поиске
		{
			name: "Database error on lookup",
			given: func(td *TestData) {
				// Given: ошибка базы данных при поиске депозита

				td.depositStore.GetByIdempotencyKeyFunc = func(ctx context.Context, key string) (*entity.Deposit, error) {
					return nil, errors.New("database connection failed")
				}

				td.idempotencyKey = "db-error-key"
			},
			when: func(td *TestData) {
				// When: вызов функции getOrCreateDeposit
				req := &DepositRequest{
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
				}

				deposit, err := td.handler.getOrCreateDeposit(td.ctx, td.idempotencyKey, req)
				td.existingDeposit = deposit
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to check idempotency key")
				assert.Nil(td.t, td.existingDeposit)
			},
		},

		// 5. Ошибка базы данных при создании
		{
			name: "Database error on create",
			given: func(td *TestData) {
				// Given: ошибка базы данных при создании депозита

				td.depositStore.GetByIdempotencyKeyFunc = func(ctx context.Context, key string) (*entity.Deposit, error) {
					return nil, entity.ErrNotFound
				}

				td.depositStore.CreateFunc = func(ctx context.Context, deposit *entity.Deposit) error {
					return errors.New("failed to create deposit")
				}

				td.idempotencyKey = "create-error-key"
			},
			when: func(td *TestData) {
				// When: вызов функции getOrCreateDeposit
				req := &DepositRequest{
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
				}

				deposit, err := td.handler.getOrCreateDeposit(td.ctx, td.idempotencyKey, req)
				td.existingDeposit = deposit
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to create deposit record")
				assert.Nil(td.t, td.existingDeposit)
			},
		},
	}

	// Запуск всех тестовых сценариев
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создание тестовых данных
			td := createTestData(t)

			// Given: установка начального состояния
			tc.given(td)

			// When: выполнение действия
			tc.when(td)

			// Then: проверка результатов
			tc.then(td, td.err)
		})
	}
}

// TestDepositFundsHandler_validateAccount проверяет функцию validateAccount
func TestDepositFundsHandler_validateAccount(t *testing.T) {
	// Определение тестовых сценариев
	testCases := []TestCase{
		// 1. Валидный активный аккаунт с соответствующей валютой
		{
			name: "Valid active account with matching currency",
			given: func(td *TestData) {
				// Given: активный аккаунт с совпадающей валютой

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return &entity.Account{
						ID:       id,
						Status:   entity.AccountStatusActive,
						Currency: "USD",
					}, nil
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.NoError(td.t, err)
				require.NotNil(td.t, td.existingAccount)
				assert.Equal(td.t, td.accountID, td.existingAccount.ID)
				assert.Equal(td.t, entity.AccountStatusActive, td.existingAccount.Status)
				assert.Equal(td.t, td.currency, td.existingAccount.Currency)
			},
		},

		// 2. Аккаунт не найден
		{
			name: "Account not found",
			given: func(td *TestData) {
				// Given: аккаунт не существует

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return nil, entity.ErrNotFound
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "account not found")
				assert.Nil(td.t, td.existingAccount)
			},
		},

		// 3. Ошибка базы данных
		{
			name: "Database error",
			given: func(td *TestData) {
				// Given: ошибка базы данных при поиске аккаунта

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return nil, errors.New("database error")
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to get account")
				assert.Nil(td.t, td.existingAccount)
			},
		},

		// 4. Неактивный аккаунт
		{
			name: "Inactive account",
			given: func(td *TestData) {
				// Given: неактивный аккаунт

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return &entity.Account{
						ID:       id,
						Status:   entity.AccountStatusInactive,
						Currency: "USD",
					}, nil
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "account is inactive")
				assert.Nil(td.t, td.existingAccount)
			},
		},

		// 5. Заблокированный аккаунт
		{
			name: "Blocked account",
			given: func(td *TestData) {
				// Given: заблокированный аккаунт

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return &entity.Account{
						ID:       id,
						Status:   entity.AccountStatusBlocked,
						Currency: "USD",
					}, nil
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "account is blocked")
				assert.Nil(td.t, td.existingAccount)
			},
		},

		// 6. Несоответствие валюты
		{
			name: "Currency mismatch",
			given: func(td *TestData) {
				// Given: валюта аккаунта не соответствует валюте депозита

				td.accountStore.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
					return &entity.Account{
						ID:       id,
						Status:   entity.AccountStatusActive,
						Currency: "EUR",
					}, nil
				}

				td.currency = "USD"
			},
			when: func(td *TestData) {
				// When: вызов функции validateAccount
				account, err := td.handler.validateAccount(td.ctx, td.accountID, td.currency)
				td.existingAccount = account
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "currency mismatch")
				assert.Nil(td.t, td.existingAccount)
			},
		},
	}

	// Запуск всех тестовых сценариев
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создание тестовых данных
			td := createTestData(t)

			// Given: установка начального состояния
			tc.given(td)

			// When: выполнение действия
			tc.when(td)

			// Then: проверка результатов
			tc.then(td, td.err)
		})
	}
}

// TestDepositFundsHandler_processDepositWithPartner проверяет функцию processDepositWithPartner
func TestDepositFundsHandler_processDepositWithPartner(t *testing.T) {
	// Определение тестовых сценариев
	testCases := []TestCase{
		// 1. Успешный депозит со статусом "В ожидании"
		{
			name: "Successful pending deposit",
			given: func(td *TestData) {
				// Given: партнерский сервис возвращает статус "В ожидании"

				td.partnerClient.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  "ext-deposit-id",
						Status: entity.DepositStatusPending,
					}, nil
				}

				td.depositStore.UpdateExternalDataFunc = func(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error {
					return nil
				}

				td.existingDeposit = &entity.Deposit{
					ID:        uuid.New(),
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
					Status:    entity.DepositStatusPending,
				}

				td.existingAccount = &entity.Account{
					ID:     uuid.New(),
					ExtID:  "ext-account-id",
					Status: entity.AccountStatusActive,
				}
			},
			when: func(td *TestData) {
				// When: вызов функции processDepositWithPartner
				result, err := td.handler.processDepositWithPartner(td.ctx, td.existingDeposit, td.existingAccount)
				td.existingDeposit = result
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.NoError(td.t, err)
				require.NotNil(td.t, td.existingDeposit)
				assert.Equal(td.t, entity.DepositStatusPending, td.existingDeposit.Status)
				assert.NotNil(td.t, td.existingDeposit.ExtID)
				assert.Equal(td.t, "ext-deposit-id", *td.existingDeposit.ExtID)

				// Проверка вызова партнерского сервиса
				require.Equal(td.t, 1, len(td.partnerClient.CreateDepositCalls()))

				// Проверка обновления данных депозита
				require.Equal(td.t, 1, len(td.depositStore.UpdateExternalDataCalls()))
			},
		},

		// 2. Успешный завершенный депозит
		{
			name: "Successful completed deposit",
			given: func(td *TestData) {
				// Given: партнерский сервис возвращает статус "Завершен"

				td.partnerClient.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  "ext-deposit-id",
						Status: entity.DepositStatusCompleted,
					}, nil
				}

				td.depositStore.UpdateExternalDataFunc = func(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error {
					return nil
				}

				td.existingDeposit = &entity.Deposit{
					ID:        uuid.New(),
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
					Status:    entity.DepositStatusPending,
				}

				td.existingAccount = &entity.Account{
					ID:     uuid.New(),
					ExtID:  "ext-account-id",
					Status: entity.AccountStatusActive,
				}
			},
			when: func(td *TestData) {
				// When: вызов функции processDepositWithPartner
				result, err := td.handler.processDepositWithPartner(td.ctx, td.existingDeposit, td.existingAccount)
				td.existingDeposit = result
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.NoError(td.t, err)
				require.NotNil(td.t, td.existingDeposit)
				assert.Equal(td.t, entity.DepositStatusCompleted, td.existingDeposit.Status)
				assert.NotNil(td.t, td.existingDeposit.ExtID)
				assert.Equal(td.t, "ext-deposit-id", *td.existingDeposit.ExtID)

				// Проверка вызова партнерского сервиса
				require.Equal(td.t, 1, len(td.partnerClient.CreateDepositCalls()))

				// Проверка обновления данных депозита
				require.Equal(td.t, 1, len(td.depositStore.UpdateExternalDataCalls()))
			},
		},

		// 3. Ошибка партнерского сервиса
		{
			name: "Partner service error",
			given: func(td *TestData) {
				// Given: партнерский сервис возвращает ошибку

				td.partnerClient.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return nil, errors.New("partner service error")
				}

				td.existingDeposit = &entity.Deposit{
					ID:        uuid.New(),
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
					Status:    entity.DepositStatusPending,
				}

				td.existingAccount = &entity.Account{
					ID:     uuid.New(),
					ExtID:  "ext-account-id",
					Status: entity.AccountStatusActive,
				}
			},
			when: func(td *TestData) {
				// When: вызов функции processDepositWithPartner
				result, err := td.handler.processDepositWithPartner(td.ctx, td.existingDeposit, td.existingAccount)
				td.existingDeposit = result
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "partner service error")
				assert.Nil(td.t, td.existingDeposit)
			},
		},

		// 4. Ошибка обновления данных
		{
			name: "Database update error",
			given: func(td *TestData) {
				// Given: ошибка при обновлении данных депозита

				td.partnerClient.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  "ext-deposit-id",
						Status: entity.DepositStatusPending,
					}, nil
				}

				td.depositStore.UpdateExternalDataFunc = func(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error {
					return errors.New("database update error")
				}

				td.existingDeposit = &entity.Deposit{
					ID:        uuid.New(),
					AccountID: td.accountID,
					Amount:    td.amount,
					Currency:  td.currency,
					Status:    entity.DepositStatusPending,
				}

				td.existingAccount = &entity.Account{
					ID:     uuid.New(),
					ExtID:  "ext-account-id",
					Status: entity.AccountStatusActive,
				}
			},
			when: func(td *TestData) {
				// When: вызов функции processDepositWithPartner
				result, err := td.handler.processDepositWithPartner(td.ctx, td.existingDeposit, td.existingAccount)
				td.existingDeposit = result
				td.err = err
			},
			then: func(td *TestData, err error) {
				// Then: проверка результатов
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to update deposit with external data")
				assert.Nil(td.t, td.existingDeposit)
			},
		},
	}

	// Запуск всех тестовых сценариев
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создание тестовых данных
			td := createTestData(t)

			// Given: установка начального состояния
			tc.given(td)

			// When: выполнение действия
			tc.when(td)

			// Then: проверка результатов
			tc.then(td, td.err)
		})
	}
}
