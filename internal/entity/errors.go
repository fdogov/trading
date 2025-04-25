package entity

import "errors"

var (
	// ErrNotFound indicates an error when an entity is not found
	ErrNotFound = errors.New("entity not found")

	// ErrDuplicateKey indicates an error when a key uniqueness is violated
	ErrDuplicateKey = errors.New("duplicate key")

	// ErrInsufficientFunds indicates an error when the balance is insufficient
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrInvalidOperation indicates an error for an invalid operation
	ErrInvalidOperation = errors.New("invalid operation")

	// ErrAccountBlocked indicates an error when the account is blocked
	ErrAccountBlocked = errors.New("account is blocked")

	// ErrAccountInactive indicates an error when the account is inactive
	ErrAccountInactive = errors.New("account is inactive")

	// ErrPartnerServiceUnavailable indicates an error when the partner service is unavailable
	ErrPartnerServiceUnavailable = errors.New("partner service unavailable")
)
