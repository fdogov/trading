package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fdogov/trading/internal/app"
	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store/postgres"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func main() {
	// Create application context
	ctx := context.Background()

	// Add trace ID for the full application lifecycle
	appTraceID := uuid.New().String()
	ctx = context.WithValue(ctx, "app_trace_id", appTraceID)

	// Load configuration
	cfg := config.LoadConfig()

	// Initialize logger
	log, err := logger.InitLogger(cfg.Logger)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer log.Sync()

	log = log.With(zap.String("app_trace_id", appTraceID))
	log.Info("Starting application")

	// Create data store
	store, err := postgres.NewStore(cfg.Database)
	if err != nil {
		log.Fatal("Failed to create store", zap.Error(err))
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Error("Failed to close store", zap.Error(err))
		}
	}()

	// Create application
	application, err := app.NewApp(
		cfg,
		store.AccountStore(),
		store.OrderStore(),
		store.DepositStore(),
		store.EventStore(),
		store.DBTransactor(),
		log,
	)
	if err != nil {
		log.Fatal("Failed to create application", zap.Error(err))
	}

	// Handle shutdown signals
	go handleSignals(application, log)

	// Start the application
	if err := application.Start(ctx); err != nil {
		log.Fatal("Failed to start application", zap.Error(err))
	}
}

// handleSignals processes application shutdown signals
func handleSignals(application *app.App, log *zap.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("Received signal, shutting down", zap.String("signal", sig.String()))
	application.Stop()
}
