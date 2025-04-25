package suite

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// BaseSuite содержит общую функциональность для всех тестовых наборов
type BaseSuite struct {
	suite.Suite
	Logger *zap.Logger
}

// SetupSuite инициализирует тестовую среду
func (s *BaseSuite) SetupSuite() {
	var err error

	// Инициализация логгера
	s.Logger, err = zap.NewDevelopment()
	s.Require().NoError(err)
}

// TearDownSuite освобождает ресурсы после завершения тестов
func (s *BaseSuite) TearDownSuite() {
	// Освобождение ресурсов
	if s.Logger != nil {
		_ = s.Logger.Sync()
	}
}

// SetupTest подготавливает тестовую среду перед каждым тестом
func (s *BaseSuite) SetupTest() {
	// Общая подготовка перед каждым тестом
}

// TearDownTest освобождает ресурсы после каждого теста
func (s *BaseSuite) TearDownTest() {
	// Общая очистка после каждого теста
}

// SkipIfShortTest пропускает тест при коротком режиме запуска (go test -short)
func SkipIfShortTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропуск компонентного теста в коротком режиме")
	}
}
