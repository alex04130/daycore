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

	// ErrUnsupported is returned when an operation is legitimate but this
	// target cannot do it — Browser.DeleteRow on a table with a composite key,
	// where "delete row X" has no meaning rather than having failed.
	//
	// Distinct from ErrNotFound on purpose: the console turns this one into an
	// explanation ("这张表没有单行 id"), and turning it into 404 would tell an
	// operator the row is gone when it is still there.
	ErrUnsupported = errors.New("unsupported for this target")

	// ErrMissingUpsertKey is returned by UpsertByCanvasID when CanvasID is empty.
	//
	// Both courses and assignments carry a UNIQUE index on (session_id,
	// canvas_id), so an empty canvas id is not "no key" — it is one specific key
	// that every keyless row shares. The upsert would then find the previous
	// keyless row and overwrite it, silently, keeping its id: two manual entries
	// collapse into one and the first is gone with no error anywhere.
	//
	// Callers that have no upstream id must mint one (see
	// server.createManualAssignment, which uses "manual:"+uuid). Refusing here
	// turns a silent data loss into a loud caller bug.
	ErrMissingUpsertKey = errors.New("upsert key is empty")
)
