package domain

import (
	"context"
	"time"
)

// Setting is one runtime configuration override.
//
// # The layering, and which half this is
//
// Every knob is classified boot or runtime (internal/config/layer.go). Boot
// knobs built something at startup — a socket, a database handle, a signing key —
// so changing the value cannot change the thing already built, and they stay
// environment-only. Runtime knobs are read fresh at each use, so a new value
// takes effect on the next read. This table is where those new values live.
//
// The environment variable is demoted to a SEED: it supplies the value until
// somebody overrides it, and after that the row wins. That ordering is what lets
// an operator change a limit from the console without editing a file and
// redeploying — which is the whole point of the console.
//
// # Why a table and not a file
//
// Two instances. A file on one machine is a setting the other instance has never
// heard of, and the console would show whichever one answered the request. The
// same reasoning already put prompt overrides and the message catalog in the
// database.
//
// # Boundaries
//
//   - A key that is not a runtime knob is REFUSED, not stored. A row for a boot
//     knob would be a value the console displays and the process ignores, which
//     is the single worst outcome available here: "I turned it off and it kept
//     doing it".
//   - Secrets are never stored here and never returned. They are boot-layer by
//     classification, so the same refusal covers them, but it is worth naming:
//     this table is readable by anything with database access, and a signing key
//     in it is a signing key in every backup.
type Setting struct {
	// Key is the Config FIELD name (not the environment variable), because the
	// field is what the code reads and what config.Apply matches against. The
	// environment variable is the operator's name for the same thing and lives
	// in the classification table.
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SettingRepository stores runtime overrides. Deployment-wide, not per session:
// these are the operator's settings, and there is exactly one operator view.
type SettingRepository interface {
	// All returns every override, for building the config snapshot at boot and
	// after each change. It is a small table read rarely, so there is no filter.
	All(ctx context.Context) ([]Setting, error)
	// Set writes one override. Storing the value as a string keeps the table
	// free of the type system: config.Apply parses it against the field, which
	// is the one place that knows what the field is.
	Set(ctx context.Context, key, value string) error
	// Delete removes an override, which restores the environment seed. That is
	// the "reset to default" the console needs, and it must be distinguishable
	// from setting the value to the empty string — for a string knob those are
	// different requests.
	Delete(ctx context.Context, key string) error
}
