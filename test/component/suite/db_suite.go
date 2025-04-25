package suite

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/store"
	"github.com/fdogov/trading/internal/store/postgres"

	// Импорт драйвера PostgreSQL для подключения к базе данных
	_ "github.com/lib/pq"
)

// DBSuite предоставляет функциональность для тестов с базой данных
type DBSuite struct {
	BaseSuite

	// DB подключения
	DB         *sqlx.DB
	DBHandler  *postgres.DB
	testSchema string

	// Конфигурация
	Config *config.Config

	// Transactor для сервисов, использующих тестовую транзакцию
	Transactor *postgres.Transactor

	// Хранилища
	AccountStore store.AccountStore
}

// SetupSuite инициализирует тестовую среду с базой данных
func (s *DBSuite) SetupSuite() {
	// Вызов родительского метода для настройки общих компонентов
	s.BaseSuite.SetupSuite()

	var err error

	// Загрузка конфигурации из config_test.yaml
	s.loadConfig()

	// Проверка существования тестовой базы данных и создание её при необходимости
	s.ensureTestDatabaseExists()

	// Подключение к существующей базе данных
	s.DB, err = s.connectToTradingDB()
	s.Require().NoError(err)

	// Создаем DBHandler для использования в тестах
	s.DBHandler, err = postgres.NewDB(s.Config.Database)
	s.Require().NoError(err)

	// Создаем Transactor для транзакций
	s.Transactor = postgres.NewTransactor(s.DBHandler)

	// Создание уникальной схемы для тестов
	testPrefix := fmt.Sprintf("test_%s_%s",
		s.T().Name()[:minInt(10, len(s.T().Name()))], // Префикс из имени теста
		uuid.New().String()[:8])                      // Уникальный суффикс
	s.testSchema = testPrefix
	err = s.createTestSchema(testPrefix)
	s.Require().NoError(err)

	// Применение миграций
	s.applyMigrations()

	// Инициализация хранилищ
	s.initRepositories()
}

// SetupTest подготавливает тестовую среду перед каждым тестом
func (s *DBSuite) SetupTest() {
	// Очистка таблиц перед каждым тестом
	s.cleanupTables()
}

// TearDownTest очищает ресурсы после каждого теста
func (s *DBSuite) TearDownTest() {
	// Дополнительная очистка
}

// TearDownSuite освобождает ресурсы после завершения тестов
func (s *DBSuite) TearDownSuite() {
	if s.DB != nil && s.testSchema != "" {
		// Удаление тестовой схемы со всеми таблицами
		s.T().Logf("Очистка тестовой схемы %s", s.testSchema)
		_, err := s.DB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", s.testSchema))
		if err != nil {
			s.T().Logf("Не удалось удалить схему: %v", err)
		}

		// Закрываем соединения
		s.DB.Close()
		if s.DBHandler != nil {
			s.DBHandler.Close()
		}
	}

	// Удаление тестовой базы данных, если она была создана нами
	dbName := s.Config.Database.DBName
	if dbName != "" && strings.Contains(dbName, "_") { // Проверяем, что это действительно тестовая база данных
		dbHost := s.Config.Database.Host
		dbPort := s.Config.Database.Port
		dbUser := s.Config.Database.User
		dbPass := s.Config.Database.Password

		// Подключение к postgres для удаления базы
		connStr := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
			dbHost, dbPort, dbUser, dbPass,
		)

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			s.T().Logf("Не удалось подключиться к postgres для удаления базы данных: %v", err)
			return
		}
		defer db.Close()

		// Проверка, что база данных существует перед удалением
		var exists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)

		if err != nil {
			s.T().Logf("Не удалось проверить существование базы данных: %v", err)
		} else if exists {
			// Закрываем все подключения к базе данных
			_, err = db.Exec(fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s'", dbName))
			if err != nil {
				s.T().Logf("Не удалось закрыть подключения к базе данных: %v", err)
			}

			// Удаляем базу данных
			_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			if err != nil {
				s.T().Logf("Не удалось удалить базу данных %s: %v", dbName, err)
			} else {
				s.T().Logf("База данных %s успешно удалена", dbName)
			}
		}
	}
}

// loadConfig загружает конфигурацию из config_test.yaml
func (s *DBSuite) loadConfig() {
	// Устанавливаем переменную окружения CONFIG_PATH, если она не задана
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		// Определяем абсолютный путь к config_test.yaml
		projectRoot, err := filepath.Abs("../../../")
		if err != nil {
			s.T().Logf("Не удалось определить корень проекта: %v", err)
			s.T().Fatalf("Ошибка инициализации конфигурации")
		}

		configTestPath := filepath.Join(projectRoot, "config", "config_test.yaml")
		if _, err := os.Stat(configTestPath); os.IsNotExist(err) {
			s.T().Logf("Файл конфигурации не найден по пути: %s", configTestPath)
			s.T().Fatalf("Ошибка инициализации конфигурации")
		}

		s.T().Logf("Используется конфигурация из: %s", configTestPath)
		err = os.Setenv("CONFIG_PATH", configTestPath)
		if err != nil {
			s.T().Logf("Не удалось установить CONFIG_PATH: %v", err)
			s.T().Fatalf("Ошибка инициализации конфигурации")
		}
	}

	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	s.Config = &cfg
	s.T().Logf("Конфигурация успешно загружена. Настройки БД: %s@%s:%s/%s",
		s.Config.Database.User, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName)
}

// ensureTestDatabaseExists проверяет существование тестовой базы данных и создает её при необходимости
func (s *DBSuite) ensureTestDatabaseExists() {
	dbHost := s.Config.Database.Host
	dbPort := s.Config.Database.Port
	dbUser := s.Config.Database.User
	dbPass := s.Config.Database.Password
	dbName := s.Config.Database.DBName

	// Строка подключения к основной базе данных postgres
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		dbHost, dbPort, dbUser, dbPass,
	)

	s.T().Logf("Проверка существования базы данных %s...", dbName)
	s.T().Logf("Используются настройки подключения: host=%s, port=%s, user=%s", dbHost, dbPort, dbUser)

	// Подключение к базе данных postgres
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		s.T().Logf("Не удалось подключиться к postgres: %v", err)
		s.T().Logf("Проверьте настройки в config_test.yaml")
		return
	}
	defer db.Close()

	// Проверка соединения с базой
	if err := db.Ping(); err != nil {
		s.T().Logf("Не удалось установить соединение с postgres: %v", err)
		s.T().Logf("Проверьте настройки в config_test.yaml или убедитесь, что сервер PostgreSQL запущен")
		return
	}

	// Проверка существования базы данных
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		s.T().Logf("Не удалось проверить существование базы данных: %v", err)
		return
	}

	// Если база данных не существует, создаем её
	if !exists {
		s.T().Logf("База данных %s не существует, создаем...", dbName)

		// В PostgreSQL нельзя использовать параметризованные запросы для имен баз данных,
		// поэтому создаем SQL строку напрямую, предварительно проверив имя базы данных
		// на наличие допустимых символов для предотвращения SQL-инъекций
		if !isValidDBName(dbName) {
			s.T().Fatalf("Некорректное имя базы данных: %s", dbName)
			return
		}

		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
		if err != nil {
			s.T().Logf("Не удалось создать базу данных: %v", err)
			return
		}

		s.T().Logf("База данных %s успешно создана", dbName)
	} else {
		s.T().Logf("База данных %s уже существует", dbName)
	}
}

// isValidDBName проверяет, что имя базы данных содержит только допустимые символы
func isValidDBName(name string) bool {
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// connectToTradingDB подключается к базе данных trading
func (s *DBSuite) connectToTradingDB() (*sqlx.DB, error) {
	dbHost := s.Config.Database.Host
	dbPort := s.Config.Database.Port
	dbUser := s.Config.Database.User
	dbPass := s.Config.Database.Password
	dbName := s.Config.Database.DBName

	s.T().Logf("Подключение к базе данных %s на %s:%s с пользователем %s", dbName, dbHost, dbPort, dbUser)

	// Создаем строку подключения
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPass, dbName, s.Config.Database.SSLMode,
	)

	var db *sqlx.DB
	var err error
	for attempts := 0; attempts < 5; attempts++ {
		db, err = sqlx.Connect("postgres", dsn)
		if err == nil {
			pingErr := db.Ping()
			if pingErr == nil {
				s.T().Logf("Успешное подключение к базе данных %s", dbName)
				return db, nil
			}
			db.Close()
			err = pingErr
		}
		s.T().Logf("Не удалось подключиться к базе данных %s попытка %d: %v, повторная попытка...", dbName, attempts+1, err)
		time.Sleep(time.Duration(attempts+1) * time.Second)
	}

	return nil, fmt.Errorf("не удалось подключиться к базе данных %s: %w", dbName, err)
}

// createTestSchema создает уникальную схему для тестов
func (s *DBSuite) createTestSchema(schemaName string) error {
	s.T().Logf("Создание тестовой схемы %s", schemaName)

	// Создание схемы
	_, err := s.DB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName))
	if err != nil {
		return fmt.Errorf("не удалось создать схему %s: %w", schemaName, err)
	}

	// Установка схемы как текущей для сессии
	_, err = s.DB.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))
	if err != nil {
		return fmt.Errorf("не удалось установить search_path для схемы %s: %w", schemaName, err)
	}

	s.T().Logf("Успешно создана и установлена схема %s", schemaName)
	return nil
}

// createMigrationScript создает скрипт миграции, если его нет
func (s *DBSuite) createMigrationScript() {
	// Определение относительного пути к директории миграций
	projectRoot, err := filepath.Abs("../../../")
	if err != nil {
		s.T().Logf("Не удалось определить корень проекта: %v", err)
		return
	}

	scriptDir := filepath.Join(projectRoot, "scripts")
	scriptPath := filepath.Join(scriptDir, "migrate.sh")

	// Если директория scripts не существует, создаем её
	if _, err := os.Stat(scriptDir); os.IsNotExist(err) {
		if err := os.MkdirAll(scriptDir, 0755); err != nil {
			s.T().Logf("Не удалось создать директорию scripts: %v", err)
			return
		}
	}

	// Если скрипт миграции не существует, создаем его
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptContent := `#!/bin/bash

set -e

# Define the path to GOPATH and GOBIN
GOPATH=${GOPATH:-$(go env GOPATH)}
GOBIN=${GOBIN:-"$GOPATH/bin"}

# Add GOBIN to PATH
export PATH="$GOBIN:$PATH"

# Check if sql-migrate is installed
if ! command -v sql-migrate &> /dev/null; then
    echo "sql-migrate is not installed, installing..."
    go install github.com/rubenv/sql-migrate/...@latest
    
    # Explicitly specify the path to the executable if it's not in PATH
    SQL_MIGRATE="$GOBIN/sql-migrate"
else
    SQL_MIGRATE="sql-migrate"
fi

# Run sql-migrate with the passed arguments
echo "Running migrations using $SQL_MIGRATE"
"$SQL_MIGRATE" "$@"
`
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			s.T().Logf("Не удалось создать скрипт миграции: %v", err)
			return
		}

		s.T().Logf("Скрипт миграции %s успешно создан", scriptPath)
	}
}

// applyMigrations применяет миграции к тестовой базе данных
func (s *DBSuite) applyMigrations() {
	// Проверяем наличие и создаем скрипт миграции, если необходимо
	s.createMigrationScript()

	// Определение относительного пути к директории миграций
	projectRoot, err := filepath.Abs("../../../")
	if err != nil {
		s.T().Logf("Не удалось определить корень проекта: %v", err)
		return
	}

	migrationsDir := filepath.Join(projectRoot, "migrations")

	// Проверка существования директории миграций
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		s.T().Fatalf("Директория миграций не найдена в %s: %v", migrationsDir, err)
		return
	}

	s.T().Logf("Использование миграций из: %s", migrationsDir)

	// Путь к конфигурационному файлу sql-migrate
	dbconfigPath := filepath.Join(projectRoot, "dbconfig.yml")
	if _, err := os.Stat(dbconfigPath); os.IsNotExist(err) {
		s.T().Fatalf("Файл dbconfig.yml не найден в %s", dbconfigPath)
		return
	}

	// Создание временного конфига для sql-migrate с нашей тестовой схемой
	tmpConfigDir, err := os.MkdirTemp("", "sql-migrate-config")
	if err != nil {
		s.T().Fatalf("Ошибка создания временной директории для конфига: %v", err)
		return
	}
	defer os.RemoveAll(tmpConfigDir)

	// Создание временного конфигурационного файла для sql-migrate
	configPath := filepath.Join(tmpConfigDir, "dbconfig.yml")
	configContent := fmt.Sprintf(`
development:
  dialect: postgres
  datasource: host=%s port=%s dbname=%s user=%s password=%s sslmode=%s search_path=%s
  dir: %s
  table: migrations
`,
		s.Config.Database.Host,
		s.Config.Database.Port,
		s.Config.Database.DBName,
		s.Config.Database.User,
		s.Config.Database.Password,
		s.Config.Database.SSLMode,
		s.testSchema,
		migrationsDir)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		s.T().Fatalf("Ошибка создания конфигурационного файла: %v", err)
		return
	}

	// Определение пути к скрипту миграции
	scriptPath := filepath.Join(projectRoot, "scripts", "migrate.sh")

	// Вывод команды для отладки
	cmdStr := fmt.Sprintf("%s up -config %s -env development", scriptPath, configPath)
	s.T().Logf("Выполнение команды миграции: %s", cmdStr)

	// Выполнение команды миграции
	cmd := exec.Command(scriptPath, "up", "-config", configPath, "-env", "development")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.T().Fatalf("Ошибка выполнения миграций: %v\nВывод: %s", err, string(output))
		return
	}

	s.T().Logf("Миграции успешно применены: %s", string(output))
}

// initRepositories инициализирует хранилища с подключением к базе данных
func (s *DBSuite) initRepositories() {
	// Инициализация хранилищ с реальной базой данных
	// Здесь подключаются реальные репозитории, а не моки
	s.AccountStore = postgres.NewAccountStore(s.DBHandler)
}

// cleanupTables очищает таблицы перед каждым тестом
func (s *DBSuite) cleanupTables() {
	// Очистка таблиц
	tables := []string{"accounts"}
	for _, table := range tables {
		_, err := s.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			s.T().Logf("Ошибка очистки таблицы %s: %v", table, err)
		}
	}
}

// getEnvOrDefault возвращает значение переменной окружения или значение по умолчанию
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// minInt возвращает минимальное из двух целых чисел
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
