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

// DepositConsumerSuite implements a set of tests for the deposit event handler
type DepositConsumerSuite struct {
	suite.DBSuite
}

// SetupSuite initializes dependencies for all tests
func (s *DepositConsumerSuite) SetupSuite() {
	s.DBSuite.SetupSuite()
}

// SetupTest чистит данные перед каждым тестом
func (s *DepositConsumerSuite) SetupTest() {

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

	// Создаем уникальный префикс для всех ID в этом тесте
	uniquePrefix := time.Now().UnixNano()

	// Generate IDs for test with additional uniqueness
	userID := fmt.Sprintf("user-%d-%s", uniquePrefix, uuid.NewString())
	extAccountID := fmt.Sprintf("ext-account-%d-%s", uniquePrefix, uuid.NewString())
	depositExtID := fmt.Sprintf("ext-deposit-%d-%s", uniquePrefix, uuid.NewString())
	currency := "USD"
	amount := decimal.NewFromInt(100)
	newBalance := decimal.NewFromInt(100)

	// Создаем уникальный ключ идемпотентности для этого конкретного теста
	idempotencyKey := fmt.Sprintf("idempotency-%d-%s", uniquePrefix, uuid.NewString())

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

				// Verify deposit was created with correct data
				deposits, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.NotNil(td.t, deposits)

				// Check deposit fields
				assert.Equal(td.t, td.accountID, deposits.AccountID)
				assert.Equal(td.t, td.amount.String(), deposits.Amount.String())
				assert.Equal(td.t, td.currency, deposits.Currency)
				assert.Equal(td.t, entity.DepositStatusPending, deposits.Status)
				assert.Equal(td.t, td.depositExtID, deposits.ExtID)

				// Check event for idempotency was stored
				event, err := td.eventStore.GetByEventID(td.ctx, td.idempotencyKey, entity.EventTypeDeposit)
				require.NoError(td.t, err)
				assert.NotNil(td.t, event)
				assert.Equal(td.t, td.idempotencyKey, event.ID)
				assert.Equal(td.t, entity.EventTypeDeposit, event.Type)

				// Verify account balance was not updated (pending deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(0).Equal(account.Balance))

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
				deposit.ExtID = td.depositExtID
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
					balance := decimal.NewFromInt(100)
					return &balance, nil
				}
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify deposit was updated with completed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status)

				// Verify account balance was updated
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(100).Equal(account.Balance))

				// Verify partner proxy client was called to get balance
				require.Equal(td.t, 1, len(td.partnerProxyAccountClientMock.GetAccountBalanceCalls()))
				assert.Equal(td.t, td.extAccountID, td.partnerProxyAccountClientMock.GetAccountBalanceCalls()[0].ExtAccountID)

				// Verify deposit producer was called to send notification
				require.Equal(td.t, 1, len(td.depositProducerMock.SendDepositEventCalls()))
				call := td.depositProducerMock.SendDepositEventCalls()[0]
				assert.Equal(td.t, deposit.ID, call.Deposit.ID)
				assert.Equal(td.t, td.userID, call.UserID)
				assert.True(td.t, decimal.NewFromInt(110).Equal(call.BalanceNew))
				assert.Equal(td.t, td.idempotencyKey, call.IdempotencyKey)

				// Check event for idempotency was stored
				event, err := td.eventStore.GetByEventID(td.ctx, td.idempotencyKey, entity.EventTypeDeposit)
				require.NoError(td.t, err)
				assert.NotNil(td.t, event)
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

				// Configure mock to return updated balance
				td.partnerProxyAccountClientMock.GetAccountBalanceFunc = func(ctx context.Context, extAccountID string) (*decimal.Decimal, error) {
					balance := td.newBalance
					return &balance, nil
				}
			},
			when: func(td *TestData) error {
				// Process the message
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Verify no error
				require.NoError(td.t, err)

				// Verify deposit was created with completed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status)
				assert.True(td.t, td.amount.Equal(deposit.Amount))
				assert.Equal(td.t, td.currency, deposit.Currency)

				// Verify account balance was updated
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, td.newBalance.Equal(account.Balance))

				// Verify partner proxy client was called to get balance
				require.Equal(td.t, 1, len(td.partnerProxyAccountClientMock.GetAccountBalanceCalls()))

				// Verify deposit producer was called to send notification
				require.Equal(td.t, 1, len(td.depositProducerMock.SendDepositEventCalls()))

				// Check event for idempotency was stored
				event, err := td.eventStore.GetByEventID(td.ctx, td.idempotencyKey, entity.EventTypeDeposit)
				require.NoError(td.t, err)
				assert.NotNil(td.t, event)
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

				// Verify deposit was created with failed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusFailed, deposit.Status)

				// Verify account balance was not updated (failed deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(0).Equal(account.Balance))

				// Verify partner proxy client was not called
				require.Equal(td.t, 0, len(td.partnerProxyAccountClientMock.GetAccountBalanceCalls()))

				// Verify deposit producer was not called (no notification for failed)
				require.Equal(td.t, 0, len(td.depositProducerMock.SendDepositEventCalls()))

				// Check event for idempotency was stored
				event, err := td.eventStore.GetByEventID(td.ctx, td.idempotencyKey, entity.EventTypeDeposit)
				require.NoError(td.t, err)
				assert.NotNil(td.t, event)
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

				// Используем полностью уникальный idempotencyKey для этого теста
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

				// Verify deposit still exists with the right data
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusCompleted, deposit.Status)

				// Account balance should still be updated (from the first processing)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, td.newBalance.Equal(account.Balance))

				// Verify the functions were not called again (due to idempotency)
				require.Equal(td.t, 0, len(td.partnerProxyAccountClientMock.GetAccountBalanceCalls()))
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
				// Verify there is a validation error
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "ext_account_id is empty")
			},
		},

		// 8. Update pending deposit to failed
		{
			name: "Update pending deposit to failed",
			given: func(td *TestData) {
				// Create account in the database using fixture
				account := fixtures.PredefinedAccounts.WithBalanceUSD(td.userID, decimal.NewFromInt(100))
				account.ExtID = td.extAccountID

				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)
				td.accountID = account.ID

				// First create a pending deposit using fixture
				deposit := fixtures.PredefinedDeposits.PendingWithAmount(td.accountID, td.amount)
				deposit.ExtID = td.depositExtID
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

				// Verify deposit was updated with failed status
				deposit, err := td.depositStore.GetByExtID(td.ctx, td.depositExtID)
				require.NoError(td.t, err)
				assert.Equal(td.t, entity.DepositStatusFailed, deposit.Status)

				// Verify account balance was not changed (failed deposits don't update balance)
				account, err := td.accountStore.GetByID(td.ctx, td.accountID)
				require.NoError(td.t, err)
				assert.True(td.t, decimal.NewFromInt(100).Equal(account.Balance))

				// Verify partner proxy client was not called
				require.Equal(td.t, 0, len(td.partnerProxyAccountClientMock.GetAccountBalanceCalls()))

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
