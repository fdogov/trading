package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fdogov/trading/internal/entity"
)

func HandlePGError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case isErrNotFound(err):
		return entity.ErrNotFound
	case isConcurrentUpdate(err):
		return fmt.Errorf("%w: %w", entity.ErrRetryExecution, err)
	case isUniqueViolatesError(err):
		return fmt.Errorf("%w: %w", entity.ErrDuplicateKey, err)
	case pgconn.Timeout(err):
		return fmt.Errorf("%w: %w", entity.ErrRetryExecution, err)
	}

	return err
}

func isErrNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows)
}

// isConcurrentUpdate: 40001 (Class 40 - Transaction Rollback: serialization_failure).
func isConcurrentUpdate(err error) bool {
	const code = "40001"
	return isMatchPGError(err, code)
}

// isUniqueViolatesError: 23505 (A violation of the constraint imposed by a unique index or a unique constraint).
func isUniqueViolatesError(err error) bool {
	const code = "23505"
	return isMatchPGError(err, code)
}

func isMatchPGError(err error, code string) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == code
}
