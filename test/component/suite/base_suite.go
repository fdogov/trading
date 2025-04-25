package suite

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// BaseSuite contains common functionality for all test suites
type BaseSuite struct {
	suite.Suite
	Logger *zap.Logger
}

// SetupSuite initializes the test environment
func (s *BaseSuite) SetupSuite() {
	var err error

	// Initialize logger
	s.Logger, err = zap.NewDevelopment()
	s.Require().NoError(err)
}

// TearDownSuite releases resources after tests are completed
func (s *BaseSuite) TearDownSuite() {
	// Release resources
	if s.Logger != nil {
		_ = s.Logger.Sync()
	}
}

// SetupTest prepares the test environment before each test
func (s *BaseSuite) SetupTest() {
	// Common preparation before each test
}

// TearDownTest releases resources after each test
func (s *BaseSuite) TearDownTest() {
	// Common cleanup after each test
}

// SkipIfShortTest skips the test in short run mode (go test -short)
func SkipIfShortTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping component test in short mode")
	}
}
