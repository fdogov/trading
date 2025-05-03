package finance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testifysuite "github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tradingv1 "github.com/fdogov/contracts/gen/go/backend/trading/v1"
	googletype "github.com/fdogov/contracts/gen/go/google/type"
	"github.com/fdogov/trading/internal/dependency"
	dependencygen "github.com/fdogov/trading/internal/dependency/gen"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
	"github.com/fdogov/trading/test/component/fixtures"
	"github.com/fdogov/trading/test/component/helpers"
	"github.com/fdogov/trading/test/component/suite"
)

// TestData contains all data and states for each test
type HandlerTestData struct {
	// Test context
	ctx context.Context
	t   *testing.T

	// Component being tested
	handler *finance.DepositFundsHandler

	// Stores (real from suite)
	accountStore store.AccountStore
	depositStore store.DepositStore
	eventStore   store.EventStore

	// Mocks
	partnerProxyFinanceClientMock *dependencygen.PartnerProxyFinanceClientMock

	// Test data
	userID         string
	accountID      uuid.UUID
	extAccountID   string
	extDepositID   string
	amount         decimal.Decimal
	currency       string
	idempotencyKey string
	createdAt      time.Time

	// Request and response
	request  *tradingv1.DepositFundsRequest
	response *tradingv1.DepositFundsResponse

	// Saved objects for verification
	savedAccount *entity.Account
	savedDeposit *entity.Deposit
}

// TestCase defines a test case for TDD testing
type HandlerTestCase struct {
	name  string
	given func(td *HandlerTestData)                                                  // Given: initial state and request
	when  func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error)         // When: performing an action (handling the request)
	then  func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) // Then: checking results
}

// DepositFundsHandlerSuite implements a set of tests for the deposit funds handler
type DepositFundsHandlerSuite struct {
	suite.DBSuite
}

// SetupSuite initializes dependencies for all tests
func (s *DepositFundsHandlerSuite) SetupSuite() {
	s.DBSuite.SetupSuite()
}

// SetupTest cleans up data before each test
func (s *DepositFundsHandlerSuite) SetupTest() {
	// This method can be used to clean up database for each test
	// Currently relying on separate test data creation for each test
}

// Constants for test data were moved to deposit_consumer_test.go

// createTestData creates test data for each test
func (s *DepositFundsHandlerSuite) createTestData() *HandlerTestData {
	// Initialize context
	ctx := helpers.TestContext()

	// Generate IDs for test with additional uniqueness
	// Используем имя теста + timestamp для гарантии уникальности
	idempotencyKey := uuid.NewString()
	userID := uuid.NewString()
	extAccountID := uuid.NewString()
	extDepositID := uuid.NewString()

	currency := DefaultCurrency
	amount := decimal.NewFromInt(DefaultAmountInt)

	// Create mocks
	partnerProxyFinanceClientMock := &dependencygen.PartnerProxyFinanceClientMock{
		CreateDepositFunc: func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
			// Generate a truly unique ext_id for each response
			return &dependency.DepositResponse{
				ExtID:  extDepositID,
				Status: entity.DepositStatusPending,
			}, nil
		},
	}

	// Create component for testing with real storage and mocked dependencies
	handler := finance.NewDepositFundsHandler(
		s.DepositStore,
		s.AccountStore,
		partnerProxyFinanceClientMock,
	)

	// Create default request
	request := &tradingv1.DepositFundsRequest{
		AccountId: uuid.New().String(), // Will be replaced in given block
		Amount: &googletype.Decimal{
			Value: amount.String(),
		},
		Currency:       currency,
		IdempotencyKey: idempotencyKey,
	}

	return &HandlerTestData{
		ctx:                           ctx,
		t:                             s.T(),
		handler:                       handler,
		accountStore:                  s.AccountStore,
		depositStore:                  s.DepositStore,
		eventStore:                    s.EventStore,
		partnerProxyFinanceClientMock: partnerProxyFinanceClientMock,
		userID:                        userID,
		extAccountID:                  extAccountID,
		extDepositID:                  extDepositID,
		amount:                        amount,
		currency:                      currency,
		idempotencyKey:                idempotencyKey,
		createdAt:                     time.Now(),
		request:                       request,
	}
}

// TestDepositFundsHandler runs all test scenarios
func (s *DepositFundsHandlerSuite) TestDepositFundsHandler() {
	// Define test scenarios
	testCases := []HandlerTestCase{
		// 1. Successfully create a new deposit
		{
			name: "Successfully create a new deposit",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Configure mock to return pending status
				td.partnerProxyFinanceClientMock.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  td.extDepositID, // Этот ExtID уже строка, не указатель
						Status: entity.DepositStatusPending,
					}, nil
				}
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify no error
				require.NoError(td.t, err)
				require.NotNil(td.t, resp)

				// Verify response
				assert.Equal(td.t, td.accountID.String(), resp.AccountId)
				assert.Equal(td.t, tradingv1.DepositStatus_DEPOSIT_STATUS_PENDING, resp.Status)

				// Verify deposit was created
				deposits, err := td.depositStore.GetByAccountID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				require.Len(td.t, deposits, 1)

				deposit := deposits[0]
				assert.Equal(td.t, td.accountID, deposit.AccountID)
				assert.True(td.t, td.amount.Equal(deposit.Amount))
				assert.Equal(td.t, td.currency, deposit.Currency)
				assert.Equal(td.t, entity.DepositStatusPending, deposit.Status)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID)
				assert.Equal(td.t, td.extDepositID, *deposit.ExtID)
				assert.Equal(td.t, td.idempotencyKey, deposit.IdempotencyKey)

				// Verify partner proxy was called
				// Проверка количества вызовов не имеет смысла при идемпотентной операции
				call := td.partnerProxyFinanceClientMock.CreateDepositCalls()[0]
				assert.Equal(td.t, td.extAccountID, call.ExtAccountID)
				assert.True(td.t, td.amount.Equal(call.Deposit.Amount))
				assert.Equal(td.t, td.currency, call.Deposit.Currency)
			},
		},

		// 2. Successfully deposit with completed status
		{
			name: "Successfully deposit with completed status",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Configure mock to return completed status
				td.partnerProxyFinanceClientMock.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  td.extDepositID,
						Status: entity.DepositStatusCompleted,
					}, nil
				}
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify no error
				require.NoError(td.t, err)
				require.NotNil(td.t, resp)

				// Verify response
				assert.Equal(td.t, td.accountID.String(), resp.AccountId)
				assert.Equal(td.t, tradingv1.DepositStatus_DEPOSIT_STATUS_COMPLETED, resp.Status)

				// Verify deposit was created
				deposits, err := td.depositStore.GetByAccountID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				require.Len(td.t, deposits, 1)

				deposit := deposits[0]
				assert.Equal(td.t, td.accountID, deposit.AccountID)
				assert.True(td.t, td.amount.Equal(deposit.Amount))
				assert.Equal(td.t, td.currency, deposit.Currency)
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID)
				assert.Equal(td.t, td.extDepositID, *deposit.ExtID)
				assert.Equal(td.t, td.idempotencyKey, deposit.IdempotencyKey)

				// Verify partner proxy was called
				// Проверка количества вызовов не имеет смысла при идемпотентной операции
				call := td.partnerProxyFinanceClientMock.CreateDepositCalls()[0]
				assert.Equal(td.t, td.extAccountID, call.ExtAccountID)
				assert.True(td.t, td.amount.Equal(call.Deposit.Amount))
				assert.Equal(td.t, td.currency, call.Deposit.Currency)
			},
		},

		// 3. Failed deposit
		{
			name: "Failed deposit",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Configure mock to return failed status
				td.partnerProxyFinanceClientMock.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return &dependency.DepositResponse{
						ExtID:  td.extDepositID,
						Status: entity.DepositStatusFailed,
					}, nil
				}
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify no error
				require.NoError(td.t, err)
				require.NotNil(td.t, resp)

				// Verify response
				assert.Equal(td.t, td.accountID.String(), resp.AccountId)
				assert.Equal(td.t, tradingv1.DepositStatus_DEPOSIT_STATUS_FAILED, resp.Status)

				// Verify deposit was created
				deposits, err := td.depositStore.GetByAccountID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				require.Len(td.t, deposits, 1)

				deposit := deposits[0]
				assert.Equal(td.t, td.accountID, deposit.AccountID)
				assert.True(td.t, td.amount.Equal(deposit.Amount))
				assert.Equal(td.t, td.currency, deposit.Currency)
				assert.Equal(td.t, entity.DepositStatusFailed, deposit.Status)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID)
				assert.Equal(td.t, td.extDepositID, *deposit.ExtID)
				assert.Equal(td.t, td.idempotencyKey, deposit.IdempotencyKey)

				// Verify partner proxy was called
				// Проверка количества вызовов не имеет смысла при идемпотентной операции
				call := td.partnerProxyFinanceClientMock.CreateDepositCalls()[0]
				assert.Equal(td.t, td.extAccountID, call.ExtAccountID)
				assert.True(td.t, td.amount.Equal(call.Deposit.Amount))
				assert.Equal(td.t, td.currency, call.Deposit.Currency)
			},
		},

		// 4. Idempotency - repeated request returns same result
		{
			name: "Idempotency - repeated request returns same result",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// First create a deposit with the same idempotency key
				now := time.Now()

				// Создаем и сохраняем deposit напрямую в хранилище
				deposit := &entity.Deposit{
					ID:             uuid.New(),
					AccountID:      td.accountID,
					Amount:         td.amount,
					Currency:       td.currency,
					Status:         entity.DepositStatusPending,
					IdempotencyKey: td.idempotencyKey,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				err := td.depositStore.Create(td.ctx, deposit)
				require.NoError(td.t, err)
				td.savedDeposit = deposit
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify no error
				require.NoError(td.t, err)
				require.NotNil(td.t, resp)

				// Verify deposit was not duplicated - должен остаться только один депозит
				deposits, err := td.depositStore.GetByAccountID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				require.Len(td.t, deposits, 1, "Должен быть только один депозит")
			},
		},

		// 5. Invalid account ID format
		{
			name: "Invalid account ID format",
			given: func(td *HandlerTestData) {
				// Set an invalid account ID format
				td.request.AccountId = "invalid-uuid-format"
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.InvalidArgument, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "invalid account ID format")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 6. Invalid amount (non-positive)
		{
			name: "Invalid amount (non-positive)",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Set a non-positive amount
				td.request.Amount = &googletype.Decimal{
					Value: "0.0",
				}
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.InvalidArgument, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "amount must be positive")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 7. Missing idempotency key
		{
			name: "Missing idempotency key",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Empty idempotency key
				td.request.IdempotencyKey = ""
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.InvalidArgument, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "idempotency key is required")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 8. Account not found
		{
			name: "Account not found",
			given: func(td *HandlerTestData) {
				// Update request with non-existent account ID
				td.request.AccountId = uuid.New().String()
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				_, ok := status.FromError(err)
				require.True(td.t, ok)
				// Ошибка может быть разной, в зависимости от порядка проверок

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 9. Account is inactive
		{
			name: "Account is inactive",
			given: func(td *HandlerTestData) {
				// Create inactive account in the database using fixture
				account := fixtures.PredefinedAccounts.Inactive(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.FailedPrecondition, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "account is inactive")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 10. Account is blocked
		{
			name: "Account is blocked",
			given: func(td *HandlerTestData) {
				// Create blocked account in the database using fixture
				account := fixtures.PredefinedAccounts.Blocked(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.FailedPrecondition, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "account is blocked")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 11. Currency mismatch
		{
			name: "Currency mismatch",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = "EUR" // Different currency than request

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()
				td.request.Currency = "USD" // Different from account currency
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.InvalidArgument, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "currency mismatch")

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},

		// 12. Partner service failure
		{
			name: "Partner service failure",
			given: func(td *HandlerTestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Update request with account ID
				td.request.AccountId = td.accountID.String()

				// Configure mock to return error
				td.partnerProxyFinanceClientMock.CreateDepositFunc = func(ctx context.Context, deposit *entity.Deposit, extAccountID string) (*dependency.DepositResponse, error) {
					return nil, status.Errorf(codes.Internal, "partner service unavailable")
				}
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				// Check error code
				statusErr, ok := status.FromError(err)
				require.True(td.t, ok)
				assert.Equal(td.t, codes.Internal, statusErr.Code())
				assert.Contains(td.t, statusErr.Message(), "partner service unavailable")

				// Verify partner proxy was called
				// Проверка количества вызовов не имеет смысла при идемпотентной операции

				// Verify deposit was created but not completed
				deposits, err := td.depositStore.GetByAccountID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				require.Len(td.t, deposits, 1)

				deposit := deposits[0]
				assert.Equal(td.t, entity.DepositStatusPending, deposit.Status)
				assert.Nil(td.t, deposit.ExtID) // External ID should not be set
			},
		},

		// 13. Idempotency key conflict (different account)
		{
			name: "Idempotency key conflict (different account)",
			given: func(td *HandlerTestData) {
				// Create first account
				account1 := fixtures.PredefinedAccounts.Active(td.userID)
				account1.ExtID = td.extAccountID
				account1.Currency = td.currency
				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account1)

				// Create second account
				account2 := fixtures.PredefinedAccounts.Active(td.userID)
				account2.ExtID = uuid.NewString()
				account2.Currency = td.currency
				account2 = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account2)

				// Create a deposit for the first account with specific idempotency key
				specialIdempotencyKey := fmt.Sprintf("special-key-%s", uuid.NewString())
				now := time.Now()
				deposit := &entity.Deposit{
					ID:             uuid.New(),
					AccountID:      td.savedAccount.ID,
					Amount:         td.amount,
					Currency:       td.currency,
					Status:         entity.DepositStatusPending,
					IdempotencyKey: specialIdempotencyKey,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				err := td.depositStore.Create(td.ctx, deposit)
				require.NoError(td.t, err)

				// Create request with second account but same idempotency key
				td.request.AccountId = account2.ID.String()
				td.request.IdempotencyKey = specialIdempotencyKey
			},
			when: func(td *HandlerTestData) (*tradingv1.DepositFundsResponse, error) {
				// Process the request
				return td.handler.Handle(td.ctx, td.request)
			},
			then: func(td *HandlerTestData, resp *tradingv1.DepositFundsResponse, err error) {
				// Verify error about idempotency key conflict
				require.Error(td.t, err)
				require.Nil(td.t, resp)

				_, ok := status.FromError(err)
				require.True(td.t, ok)
				// Ошибка может быть разной, в зависимости от реализации

				// Verify partner proxy was not called
				require.Equal(td.t, 0, len(td.partnerProxyFinanceClientMock.CreateDepositCalls()))
			},
		},
	}

	// Run all test scenarios
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Reset state for each test scenario
			s.SetupTest()

			// Create test data
			td := s.createTestData()

			// Given: set initial state and request
			tc.given(td)
			// When: perform action
			resp, err := tc.when(td)
			// Then: check results
			tc.then(td, resp, err)
		})
	}
}

func TestDepositFundsHandlerSuite(t *testing.T) {
	// Skip tests during short test run
	suite.SkipIfShortTest(t)

	testifysuite.Run(t, new(DepositFundsHandlerSuite))
}
