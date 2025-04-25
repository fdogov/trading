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

// TestData содержит все данные и состояния для каждого теста
type TestData struct {
	// Контекст теста
	ctx context.Context
	t   *testing.T

	// Компонент, который тестируем
	consumer *accounts.AccountConsumer

	// Хранилище (реальное из suite)
	accountStore store.AccountStore

	// Тестовые данные
	userID       string
	extAccountID string
	currency     string

	// Сохраненные объекты для проверки
	savedAccount *entity.Account

	// Сообщение для обработки
	message []byte
	error   error
}

// TestCase определяет тест-кейс для TDD тестирования
type TestCase struct {
	name  string
	given func(td *TestData)            // Given: начальное состояние и сообщение
	when  func(td *TestData) error      // When: выполнение действия (обработка сообщения)
	then  func(td *TestData, err error) // Then: проверка результатов
}

// AccountConsumerSuite реализует набор тестов для обработчика событий аккаунтов
type AccountConsumerSuite struct {
	suite.DBSuite
	testLogger *zap.Logger
}

// SetupSuite инициализирует зависимости для всех тестов
func (s *AccountConsumerSuite) SetupSuite() {
	s.DBSuite.SetupSuite()

	// Инициализируем тестовый логгер
	s.testLogger = zaptest.NewLogger(s.T())
}

// createAccountEvent создает стандартное событие создания аккаунта
func createAccountEvent(userID, extAccountID, currency string) []byte {
	event := originationkafkav1.AccountEvent{
		UserId:       userID,
		ExtAccountId: extAccountID,
		Currency:     currency,
	}

	data, _ := json.Marshal(event)
	return data
}

// createTestData создает тестовые данные для каждого теста
func (s *AccountConsumerSuite) createTestData() *TestData {
	// Инициализация контекста
	ctx := helpers.TestContext()

	// Генерация ID для теста
	userID := uuid.NewString()
	extAccountID := uuid.NewString()
	currency := "USD"

	// Создание компонента для тестирования с реальным хранилищем
	consumer := accounts.NewAccountConsumer(s.AccountStore, s.testLogger.Named("account_consumer"))

	return &TestData{
		ctx:          ctx,
		t:            s.T(),
		consumer:     consumer,
		accountStore: s.AccountStore,
		userID:       userID,
		extAccountID: extAccountID,
		currency:     currency,
		// Сообщение будет установлено в блоке given
	}
}

// TestAccountConsumer запускает все тестовые сценарии
func (s *AccountConsumerSuite) TestAccountConsumer() {
	// Определение тестовых сценариев
	testCases := []TestCase{
		// 1. Успешное создание аккаунта
		{
			name: "Успешное создание аккаунта",
			given: func(td *TestData) {
				// Given: создаем событие для обработки
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: вызываем обработчик события
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: проверяем результаты
				require.NoError(td.t, err)

				// Получаем аккаунт из БД для проверки
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

		// 2. Повторное получение того же события (идемпотентность)
		{
			name: "Повторное получение того же события (идемпотентность)",
			given: func(td *TestData) {
				// Given: аккаунт уже существует
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

				// Создаем сообщение с тем же extAccountID
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: вызываем обработчик события повторно
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: операция должна завершиться успешно (идемпотентность)
				require.NoError(td.t, err)

				// Проверяем, что не был создан новый аккаунт
				accounts, err := td.accountStore.GetByUserID(td.ctx, td.userID)
				require.NoError(td.t, err)
				assert.Len(td.t, accounts, 1, "Должен быть только один аккаунт")
				assert.Equal(td.t, td.savedAccount.ID, accounts[0].ID, "ID аккаунта не должен измениться")
			},
		},

		// 3. Ошибка десериализации сообщения
		{
			name: "Ошибка десериализации сообщения",
			given: func(td *TestData) {
				// Given: некорректное сообщение
				td.message = []byte(`{"invalid_json": ]`)
			},
			when: func(td *TestData) error {
				// When: вызываем обработчик события
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: должна быть ошибка десериализации
				require.Error(td.t, err)
				assert.Contains(td.t, err.Error(), "failed to unmarshal")
			},
		},

		// 4. Создание аккаунта с другой валютой
		{
			name: "Создание аккаунта с другой валютой",
			given: func(td *TestData) {
				// Given: создаем событие с другой валютой
				td.currency = "EUR"
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: вызываем обработчик события
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: проверяем результаты
				require.NoError(td.t, err)

				// Получаем аккаунт из БД для проверки
				account, err := td.accountStore.GetByExtID(td.ctx, td.extAccountID)
				require.NoError(td.t, err)
				assert.NotNil(td.t, account)
				assert.Equal(td.t, td.currency, account.Currency)
			},
		},

		// 5. Создание аккаунта для существующего пользователя (второй аккаунт)
		{
			name: "Создание второго аккаунта для существующего пользователя",
			given: func(td *TestData) {
				// Given: у пользователя уже есть один аккаунт
				existingAccount := &entity.Account{
					ID:        uuid.New(),
					UserID:    td.userID,
					ExtID:     uuid.NewString(), // Другой внешний ID
					Balance:   decimal.NewFromInt(0),
					Currency:  "USD",
					Status:    entity.AccountStatusActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				helpers.CreateAccountWithParams(td.ctx, td.t, td.accountStore, existingAccount)

				// Создаем сообщение для нового аккаунта с тем же userID
				td.message = createAccountEvent(td.userID, td.extAccountID, td.currency)
			},
			when: func(td *TestData) error {
				// When: вызываем обработчик события
				return td.consumer.ProcessMessage(td.ctx, td.message)
			},
			then: func(td *TestData, err error) {
				// Then: операция должна завершиться успешно
				require.NoError(td.t, err)

				// Проверяем, что создан второй аккаунт
				accounts, err := td.accountStore.GetByUserID(td.ctx, td.userID)
				require.NoError(td.t, err)
				assert.Len(td.t, accounts, 2, "Должно быть два аккаунта")

				// Проверяем, что новый аккаунт существует
				found := false
				for _, acc := range accounts {
					if acc.ExtID == td.extAccountID {
						found = true
						break
					}
				}
				assert.True(td.t, found, "Новый аккаунт должен быть создан")
			},
		},
	}

	// Запуск всех тестовых сценариев
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Сбрасываем состояние для каждого тестового сценария
			s.SetupTest()

			// Создаем тестовые данные
			td := s.createTestData()

			// Given: устанавливаем начальное состояние и сообщение
			tc.given(td)
			// When: выполняем действие
			err := tc.when(td)
			// Then: проверяем результаты
			tc.then(td, err)
		})
	}
}

func TestAccountConsumerSuite(t *testing.T) {
	// Пропускаем тесты при коротком запуске тестов
	suite.SkipIfShortTest(t)

	testifysuite.Run(t, new(AccountConsumerSuite))
}
