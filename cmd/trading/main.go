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
	// Создаем контекст приложения
	ctx := context.Background()

	// Добавляем trace ID для полного цикла работы приложения
	appTraceID := uuid.New().String()
	ctx = context.WithValue(ctx, "app_trace_id", appTraceID)

	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	// Инициализируем логгер
	log, err := logger.InitLogger(cfg.Logger)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer log.Sync()

	log = log.With(zap.String("app_trace_id", appTraceID))
	log.Info("Starting application")

	// Создаем хранилище данных
	store, err := postgres.NewStore(cfg.Database)
	if err != nil {
		log.Fatal("Failed to create store", zap.Error(err))
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Error("Failed to close store", zap.Error(err))
		}
	}()

	// Создаем приложение
	application, err := app.NewApp(
		cfg,
		store.AccountStore(),
		store.OrderStore(),
		store.DepositStore(),
		store.DBTransactor(),
		log,
	)
	if err != nil {
		log.Fatal("Failed to create application", zap.Error(err))
	}

	// Обрабатываем сигналы остановки
	go handleSignals(application, log)

	// Запускаем приложение
	if err := application.Start(ctx); err != nil {
		log.Fatal("Failed to start application", zap.Error(err))
	}
}

// handleSignals обрабатывает сигналы остановки приложения
func handleSignals(application *app.App, log *zap.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("Received signal, shutting down", zap.String("signal", sig.String()))
	application.Stop()
}
