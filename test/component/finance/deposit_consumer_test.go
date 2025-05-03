package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testifysuite "github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/timestamppb"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	googletype "github.com/fdogov/contracts/gen/go/google/type"
	dependencygen "github.com/fdogov/trading/internal/dependency/gen"
	"github.com/fdogov/trading/internal/domain/finance"
	"github.com/fdogov/trading/internal/entity"
	produsersgen "github.com/fdogov/trading/internal/producers/gen"
	"github.com/fdogov/trading/internal/store"
	"github.com/fdogov/trading/test/component/fixtures"
	"github.com/fdogov/trading/test/component/helpers"
	"github.com/fdogov/trading/test/component/suite"
)

// TestData contains all data and states for each test
type TestData struct {
	// Test context
	ctx context.Context
	t   *testing.T

	// Component being tested
	consumer *finance.DepositConsumer

	// Stores (real from suite)
	accountStore store.AccountStore
	depositStore store.DepositStore
	eventStore   store.EventStore

	// Mocks
	depositProducerMock           *produsersgen.DepositProducerIMock
	partnerProxyAccountClientMock *dependencygen.PartnerProxyAccountClientMock

	// Test data
	userID         string
	accountID      uuid.UUID
	extAccountID   string
	depositExtID   string
	amount         decimal.Decimal
	currency       string
	idempotencyKey string
	createdAt      time.Time
	newBalance     decimal.Decimal

	// Saved objects for verification
	savedAccount *entity.Account
	savedDeposit *entity.Deposit

	// Message to process
	message []byte
	error   error
}

// TestCase defines a test case for TDD testing
type TestCase struct {
	name  string
	given func(td *TestData)            // Given: initial state and message
	when  func(td *TestData) error      // When: performing an action (message processing)
	then  func(td *TestData, err error) // Then: checking results
}

// Constants for test data
const (
	DefaultCurrency  = "USD"
	DefaultAmountInt = 100
)

// DepositConsumerSuite implements a set of tests for the deposit event handler
type DepositConsumerSuite struct {
	suite.DBSuite
}

// SetupSuite initializes dependencies for all tests
func (s *DepositConsumerSuite) SetupSuite() {
	s.DBSuite.SetupSuite()
}

// SetupTest cleans up data before each test
func (s *DepositConsumerSuite) SetupTest() {
	// This method can be used to clean up database for each test
	// Currently relying on separate test data creation
}

// createDepositEvent creates a deposit event for testing
func createDepositEvent(
	extID, extAccountID, currency string,
	amount decimal.Decimal,
	newBalance decimal.Decimal,
	status partnerconsumerkafkav1.DepositStatus,
	idempotencyKey string,
) []byte {
	createdAt := timestamppb.Now()

	// Create the Amount proto message
	amountValue := &googletype.Decimal{
		Value: amount.String(),
	}

	// Create the BalanceNew proto message
	var balanceNewValue *googletype.Decimal
	if !newBalance.Equal(decimal.Zero) {
		balanceNewValue = &googletype.Decimal{
			Value: newBalance.String(),
		}
	}

	// Create the deposit event
	event := partnerconsumerkafkav1.DepositEvent{
		ExtId:          extID,
		ExtAccountId:   extAccountID,
		Amount:         amountValue,
		Currency:       currency,
		Status:         status,
		BalanceNew:     balanceNewValue,
		CreatedAt:      createdAt,
		IdempotencyKey: idempotencyKey,
	}

	data, _ := json.Marshal(event)
	return data
}

// createTestData creates test data for each test
func (s *DepositConsumerSuite) createTestData() *TestData {
	// Initialize context
	ctx := helpers.TestContext()

	// Generate IDs for test with additional uniqueness
	idempotencyKey := uuid.NewString()
	userID := uuid.NewString()
	extAccountID := uuid.NewString()
	depositExtID := uuid.NewString()

	currency := DefaultCurrency
	amount := decimal.NewFromInt(DefaultAmountInt)
	newBalance := decimal.NewFromInt(DefaultAmountInt)

	// Create mocks
	depositProducerMock := &produsersgen.DepositProducerIMock{
		SendDepositEventFunc: func(ctx context.Context, deposit *entity.Deposit, userID string, balanceNew decimal.Decimal, idempotencyKey string) error {
			return nil
		},
	}

	partnerProxyAccountClientMock := &dependencygen.PartnerProxyAccountClientMock{
		GetAccountBalanceFunc: func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
			balance := newBalance
			return &balance, nil
		},
	}

	// Create component for testing with real storage and mocked dependencies
	consumer := finance.NewDepositConsumer(
		s.DepositStore,
		s.AccountStore,
		s.EventStore,
		depositProducerMock,
		partnerProxyAccountClientMock,
	)

	return &TestData{
		ctx:                           ctx,
		t:                             s.T(),
		consumer:                      consumer,
		accountStore:                  s.AccountStore,
		depositStore:                  s.DepositStore,
		eventStore:                    s.EventStore,
		depositProducerMock:           depositProducerMock,
		partnerProxyAccountClientMock: partnerProxyAccountClientMock,
		userID:                        userID,
		extAccountID:                  extAccountID,
		depositExtID:                  depositExtID,
		amount:                        amount,
		currency:                      currency,
		newBalance:                    newBalance,
		idempotencyKey:                idempotencyKey,
		createdAt:                     time.Now(),
		// Message will be set in the given block
	}
}

// TestDepositConsumer runs all test scenarios
func (s *DepositConsumerSuite) TestDepositConsumer() {
	// Define test scenarios
	testCases := []TestCase{
		// 1. Process new pending deposit
		{
			name: "Process new pending deposit",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Create a pending deposit event
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					decimal.Zero, // No balance update for pending
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_PENDING,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify account balance was not updated (pending deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(0).Equal(account.Balance))

				// Verify deposit was created with pending status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID, "Deposit external ID should be set")
				assert.Equal(td.t, td.depositExtID, *deposit.ExtID, "Deposit external ID should match")
				assert.Equal(td.t, entity.DepositStatusPending, deposit.Status, "Deposit should be in pending status")
				assert.True(td.t, td.amount.Equal(deposit.Amount), "Deposit amount should match")
				assert.Equal(td.t, td.currency, deposit.Currency, "Deposit currency should match")

				// Verify deposit producer was not called (no notification for pending)
				assert.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))
			},
		},

		// 2. Update deposit from pending to completed
		{
			name: "Update deposit from pending to completed",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.Active(td.userID)
				account.ExtID = td.extAccountID
				account.Currency = td.currency

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// First create a pending deposit using fixture
				deposit := fixtures.PredefinedDeposits.Pending(td.accountID)
				// Создаем копию строки для корректного присваивания указателю
				extID := td.depositExtID
				deposit.ExtID = &extID
				deposit.Amount = td.amount
				deposit.Currency = td.currency

				err := td.depositStore.Create(td.ctx, deposit)
				require.NoError(td.t, err)
				td.savedDeposit = deposit

				// Create a completed deposit event (updating from pending)
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					decimal.NewFromInt(110), // Now with balance update
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Configure mock to return updated balance
				td.partnerProxyAccountClientMock.GetAccountBalanceFunc = func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
					balance := decimal.NewFromInt(DefaultAmountInt)
					return &balance, nil
				}
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify account balance was updated
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(DefaultAmountInt).Equal(account.Balance))

				// Verify deposit status was updated to completed
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status, "Deposit should be updated to completed status")
				assert.True(td.t, td.amount.Equal(deposit.Amount), "Deposit amount should remain unchanged")
				assert.Equal(td.t, td.currency, deposit.Currency, "Deposit currency should remain unchanged")

				// Verify deposit producer was called to send notification
				require.Equal(td.t, 1, len(td.depositProducerMock.SendDepositEventCalls()))
				call := td.depositProducerMock.SendDepositEventCalls()[0]
				assert.Equal(td.t, td.userID, call.UserID)
				assert.True(td.t, decimal.NewFromInt(110).Equal(call.BalanceNew))
				assert.Equal(td.t, td.idempotencyKey, call.IdempotencyKey)
			},
		},

		// 3. Receive completed deposit directly (no prior pending)
		{
			name: "Receive completed deposit directly",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.WithBalanceUSD(td.userID, decimal.NewFromInt(0))
				account.ExtID = td.extAccountID

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Create a completed deposit event directly
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					td.newBalance,
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Configure mock to return updated balance
				td.partnerProxyAccountClientMock.GetAccountBalanceFunc = func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
					balance := td.newBalance
					return &balance, nil
				}

				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify account balance was updated
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, td.newBalance.Equal(account.Balance))

				// Verify deposit was created with completed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID, "Deposit external ID should be set")
				assert.Equal(td.t, td.depositExtID, *deposit.ExtID, "Deposit external ID should match")
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status, "Deposit should be in completed status")
				assert.True(td.t, td.amount.Equal(deposit.Amount), "Deposit amount should match")
				assert.Equal(td.t, td.currency, deposit.Currency, "Deposit currency should match")

				// Verify deposit producer was called to send notification
				require.Equal(td.t, 1, len(td.depositProducerMock.SendDepositEventCalls()))
				call := td.depositProducerMock.SendDepositEventCalls()[0]
				assert.Equal(td.t, td.userID, call.UserID)
				assert.True(td.t, decimal.NewFromInt(DefaultAmountInt).Equal(call.BalanceNew),
					fmt.Sprintf("BalanceNew should be %d", DefaultAmountInt), call.BalanceNew)
				assert.Equal(td.t, td.idempotencyKey, call.IdempotencyKey)
			},
		},

		// 4. Receive failed deposit
		{
			name: "Receive failed deposit",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.WithBalanceUSD(td.userID, decimal.NewFromInt(0))
				account.ExtID = td.extAccountID

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Create a failed deposit event
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					decimal.Zero, // Failed deposits don't update balance
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_FAILED,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify account balance was not updated (failed deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(0).Equal(account.Balance))

				// Verify deposit was created with failed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				// Проверяем, что ExtID установлен и его значение совпадает с ожидаемым
				assert.NotNil(td.t, deposit.ExtID, "Deposit external ID should be set")
				assert.Equal(td.t, td.depositExtID, *deposit.ExtID, "Deposit external ID should match")
				assert.Equal(td.t, entity.DepositStatusFailed, deposit.Status, "Deposit should be in failed status")
				assert.True(td.t, td.amount.Equal(deposit.Amount), "Deposit amount should match")
				assert.Equal(td.t, td.currency, deposit.Currency, "Deposit currency should match")

				// Verify deposit producer was not called (no notification for failed)
				require.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))
			},
		},

		// 5. Idempotency - receiving the same event twice
		{
			name: "Idempotency - receiving the same event twice",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.WithBalanceUSD(td.userID, decimal.NewFromInt(0))
				account.ExtID = td.extAccountID

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// Use a completely unique idempotencyKey для этого теста
				testSpecificIdempotencyKey := fmt.Sprintf("idempotency-test-%d-%s", time.Now().UnixNano(), uuid.NewString())
				td.idempotencyKey = testSpecificIdempotencyKey

				// Create a deposit event
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					td.newBalance,
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED,
					testSpecificIdempotencyKey, // Явно передаем тот же ключ
				)

				// Configure mock to return updated balance
				td.partnerProxyAccountClientMock.GetAccountBalanceFunc = func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
					balance := td.newBalance
					return &balance, nil
				}

				// Process it once
				err := td.consumer.ProcessMessage(td.ctx, td.message)
				require.NoError(td.t, err)

				// Создаем новые моки для второго вызова, чтобы отслеживать вызовы отдельно
				// Reset mocks for the second processing
				td.depositProducerMock = &produsersgen.DepositProducerIMock{
					SendDepositEventFunc: func(ctx context.Context, deposit *entity.Deposit, userID string, balanceNew decimal.Decimal, idempotencyKey string) error {
						return nil
					},
				}

				td.partnerProxyAccountClientMock = &dependencygen.PartnerProxyAccountClientMock{
					GetAccountBalanceFunc: func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
						balance := td.newBalance
						return &balance, nil
					},
				}

				// Пересоздаем консьюмер с новыми моками
				td.consumer = finance.NewDepositConsumer(
					td.depositStore,
					td.accountStore,
					td.eventStore,
					td.depositProducerMock,
					td.partnerProxyAccountClientMock,
				)
			},
			when: func(td *TestData) error {
				// Process the same message again
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error (idempotency should work silently)
				require.NoError(td.t, err)

				// Account balance should still be updated (from the first processing)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, td.newBalance.Equal(account.Balance))

				require.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))
			},
		},

		// 6. Invalid message format
		{
			name: "Invalid message format",
			given: func(td *TestData) {
				// Create an invalid message (not valid JSON)
				td.message = []byte(`{"invalid_json": ]`)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify there is an error
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "unmarshal")
			},
		},

		// 7. Missing required fields
		{
			name: "Missing required fields",
			given: func(td *TestData) {
				// Create a message with missing required fields
				event := partnerconsumerkafkav1.DepositEvent{
					ExtId: td.depositExtID,
					// Missing ExtAccountId
					Currency:  td.currency,
					Status:    partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED,
					CreatedAt: timestamppb.Now(),
				}
				td.message, _ = json.Marshal(event)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify there is a specific validation error
				require.Error(td.t, err, "Processing message with missing required fields should return an error")
				assert.Contains(td.t, err.Error(), "ext_account_id is empty",
					"Error should specifically mention the missing ext_account_id field")
			},
		},

		// 8. Non-existent account - deposit to unknown account
		{
			name: "Non-existent account - deposit to unknown account",
			given: func(td *TestData) {
				// Use a non-existent account external ID
				nonExistentExtAccountID := fmt.Sprintf("non-existent-account-%s", uuid.NewString())

				// Create a deposit event for a non-existent account
				td.message = createDepositEvent(
					td.depositExtID,
					nonExistentExtAccountID, // Use non-existent account ID
					td.currency,
					td.amount,
					td.newBalance,
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify there is an error about non-existent account
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "entity not found",
					"Error should mention that account was not found")
				assert.Contains(td.t, err.Error(), "failed to find account",
					"Error should mention that account was not found")

				// Verify no deposit was created
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				assert.Error(td.t, err, "Should return error when deposit doesn't exist")
				assert.Nil(td.t, deposit, "No deposit should be created for non-existent account")

				// Attempt to find by another method to be doubly sure
				deposit, err = td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.Nil(td.t, deposit, "Deposit should be created for non-existent account")
				require.Error(td.t, entity.ErrNotFound)

				// Verify deposit producer was not called
				require.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))
			},
		},

		// 9. Update pending deposit to failed
		{
			name: "Update pending deposit to failed",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.WithBalanceUSD(td.userID, decimal.NewFromInt(150))
				account.ExtID = td.extAccountID

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// First create a pending deposit using fixture
				deposit := fixtures.PredefinedDeposits.PendingWithAmount(td.accountID, td.amount)
				// Создаем копию строки для корректного присваивания указателю
				extID := td.depositExtID
				deposit.ExtID = &extID
				deposit.Currency = td.currency

				err := td.depositStore.Create(td.ctx, deposit)
				require.NoError(td.t, err)
				td.savedDeposit = deposit

				// Create a failed deposit event (updating from pending)
				td.message = createDepositEvent(
					td.depositExtID,
					td.extAccountID,
					td.currency,
					td.amount,
					decimal.Zero, // No balance update for failed
					partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_FAILED,
					td.idempotencyKey,
				)
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify account balance was not changed (failed deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(150).Equal(account.Balance))

				// Verify deposit status was updated to failed
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusFailed, deposit.Status, "Deposit should be updated to failed status")
				assert.True(td.t, td.amount.Equal(deposit.Amount), "Deposit amount should remain unchanged")
				assert.Equal(td.t, td.currency, deposit.Currency, "Deposit currency should remain unchanged")

				// Verify deposit producer was not called (no notification for failed)
				require.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))
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

			// Given: set initial state and message
			tc.given(td)
			// When: perform action
			err := tc.when(td)
			// Then: check results
			tc.then(td, err)
		})
	}
}

func TestDepositConsumerSuite(t *testing.T) {
	// Skip tests during short test run
	suite.SkipIfShortTest(t)

	testifysuite.Run(t, new(DepositConsumerSuite))
}
