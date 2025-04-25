package entity

import "errors"

var (
	// ErrNotFound отображает ошибку, когда сущность не найдена
	ErrNotFound = errors.New("entity not found")
	
	// ErrDuplicateKey отображает ошибку, когда нарушается уникальность ключа
	ErrDuplicateKey = errors.New("duplicate key")
	
	// ErrInsufficientFunds отображает ошибку при недостаточном балансе
	ErrInsufficientFunds = errors.New("insufficient funds")
	
	// ErrInvalidOperation отображает ошибку при недопустимой операции
	ErrInvalidOperation = errors.New("invalid operation")
	
	// ErrAccountBlocked отображает ошибку когда аккаунт заблокирован
	ErrAccountBlocked = errors.New("account is blocked")
	
	// ErrAccountInactive отображает ошибку когда аккаунт неактивен
	ErrAccountInactive = errors.New("account is inactive")
	
	// ErrPartnerServiceUnavailable отображает ошибку когда сервис партнера недоступен
	ErrPartnerServiceUnavailable = errors.New("partner service unavailable")
)
