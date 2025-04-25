package accounts

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testifysuite "github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	originationkafkav1 "github.com/fdogov/contracts/gen/go/backend/origination/kafka/v1"
	"github.com/fdogov/trading/internal/domain/accounts"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
	"github.com/fdogov/trading/test/component/helpers"
	"github.com/fdogov/trading/test/component/suite"
)

// TestData contains all data and states for each test
type TestData struct {
	// Test context
	ctx context.Context
	t   *testing.T

	// Component being tested
	consumer *accounts.AccountConsumer

	// Store (real from suite)
	accountStore store.AccountStore

	// Test data
	userID       string
	extAccountID string
	currency     string

	// Saved objects for verification
	savedAccount *entity.Account

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

// AccountConsumerSuite implements a set of tests for the account event handler
type AccountConsumerSuite struct {
	suite.DBSuite
	testLogger *zap.Logger
}

// SetupSuite initializes dependencies for all tests
func (s *AccountConsumerSuite) SetupSuite() {
	s.DBSuite.SetupSuite()

	// Initialize test logger
	s.testLogger = zaptest.NewLogger(s.T())
}

// createAccountEvent creates a standard account creation event
func createAccountEvent(userID, extAccountID, currency string) []byte {
	event := originationkafkav1.AccountEvent{
		UserId:       userID,
		ExtAccountId: extAccountID,
		Currency:     currency,
	}

	data, _ := json.Marshal(event)
	return data
}

// createTestData creates test data for each test
func (s *AccountConsumerSuite) createTestData() *TestData {
	// Initialize context
	ctx := helpers.TestContext()

	// Generate IDs for test
	userID := uuid.NewString()
	extAccountID := uuid.NewString()
	currency := "USD"

	// Create component for testing with real storage
	consumer := accounts.NewAccountConsumer(s.AccountStore, s.testLogger.Named("account_consumer"))

	return &TestData{
		ctx:          ctx,
		t:            s.T(),
		consumer:     consumer,
		accountStore: s.AccountStore,
		userID:       userID,
		extAccountID: extAccountID,
		currency:     currency,
		// Message will be set in the given block
	}
}

// TestAccountConsumer runs all test scenarios
func (s *AccountConsumerSuite) TestAccountConsumer() {
	// Define test scenarios
	testCases := []TestCase{
		// 1. Successful account creation
		{
			name: "Successful account creation",
			given: func(td *TestData) {
				// Given: create an event for processing
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: call the event handler
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: check the results
				require.NoError(td.t, err)

				// Get the account from DB for verification
				account, err := td.accountStore.GetByExtID(td.ctx, td.extAccountID)
				require.NoError(td.t, err)
				assert.NotNil(td.t, account)
				assert.Equal(td.t, td.userID, account.UserID)
				assert.Equal(td.t, td.extAccountID, account.ExtID)
				assert.Equal(td.t, entity.AccountStatusActive, account.Status)
				assert.Equal(td.t, td.currency, account.Currency)
				assert.True(td.t, account.Balance.Equal(decimal.NewFromInt(0)))
			},
		},

		// 2. Receiving the same event again (idempotence)
		{
			name: "Receiving the same event again (idempotence)",
			given: func(td *TestData) {
				// Given: account already exists
				account := &entity.Account{
					ID:        uuid.New(),
					UserID:    td.userID,
					ExtID:     td.extAccountID,
					Balance:   decimal.NewFromInt(0),
					Currency:  td.currency,
					Status:    entity.AccountStatusActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				td.savedAccount = helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, account)

				// Create message with the same extAccountID
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: call the event handler again
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: operation should complete successfully (idempotence)
				require.NoError(td.t, err)

				// Check that no new account was created
				accounts, err := td.accountStore.GetByUserID(td.ctx, td.userID)
				require.NoError(td.t, err)
				assert.Len(td.t, accounts, 1, "There should be only one account")
				assert.Equal(td.t, td.savedAccount.ID, accounts[0].ID, "Account ID should not change")
			},
		},

		// 3. Message deserialization error
		{
			name: "Message deserialization error",
			given: func(td *TestData) {
				// Given: invalid message
				td.message = []byte(`{"invalid_json": ]`)
			},
			when: func(td *TestData) error {
				// When: call the event handler
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: there should be a deserialization error
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to unmarshal")
			},
		},

		// 4. Creating an account with different currency
		{
			name: "Creating an account with different currency",
			given: func(td *TestData) {
				// Given: create an event with different currency
				td.currency = "EUR"
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: call the event handler
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: check the results
				require.NoError(td.t, err)

				// Get the account from DB for verification
				account, err := td.accountStore.GetByExtID(td.ctx, td.extAccountID)
				require.NoError(td.t, err)
				assert.NotNil(td.t, account)
				assert.Equal(td.t, td.currency, account.Currency)
			},
		},

		// 5. Creating a second account for an existing user
		{
			name: "Creating a second account for an existing user",
			given: func(td *TestData) {
				// Given: user already has one account
				existingAccount := &entity.Account{
					ID:        uuid.New(),
					UserID:    td.userID,
					ExtID:     uuid.NewString(), // Different external ID
					Balance:   decimal.NewFromInt(0),
					Currency:  "USD",
					Status:    entity.AccountStatusActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, existingAccount)

				// Create message for a new account with the same userID
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: call the event handler
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: operation should complete successfully
				require.NoError(td.t, err)

				// Check that a second account was created
				accounts, err := td.accountStore.GetByUserID(td.ctx, td.userID)
				require.NoError(td.t, err)
				assert.Len(td.t, accounts, 2, "There should be two accounts")

				// Check that the new account exists
				found := false
				for _, acc := range accounts {
					if acc.ExtID == td.extAccountID {
						found = true
						break
					}
				}
				assert.True(td.t, found, "New account should be created")
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

func TestAccountConsumerSuite(t *testing.T) {
	// Skip tests during short test run
	suite.SkipIfShortTest(t)

	testifysuite.Run(t, new(AccountConsumerSuite))
}
