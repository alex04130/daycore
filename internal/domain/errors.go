package domain

import "errors"

// Sentinel errors returned by Store implementations. Handlers map these to HTTP
// status codes, so every adapter (SQL or Mongo) must return these exact values
// rather than driver-specific errors.
var (
	// ErrNotFound is returned when a requested row/document does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned on a unique-constraint violation (e.g. email taken).
	ErrConflict = errors.New("conflict")
	// ErrInvalidCredentials is returned when an email/password pair is wrong.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnsupportedDBType is returned by storage.Open for an unregistered driver.
	ErrUnsupportedDBType = errors.New("unsupported database type")
)
