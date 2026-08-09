package domain

import (
	"context"
	"time"
)

// ProviderOverride is the console-editable half of one external capability
// source (a weather source, a search source, a messaging channel).
//
// # The split, and the rule that produced it
//
// A source is described in two places, and the line between them is the same
// one F1 drew across the environment variables: **did the process already build
// something out of it?**
//
//	config/providers.yaml   id, format, base_url, token_env, capabilities
//	provider_overrides      enabled, description, approved
//
// The file half is boot-layer — the process constructed an HTTP client from
// base_url at startup, so a new value cannot change the thing already built,
// exactly like DB_DSN. The table half is read fresh at each use, so a new value
// takes effect on the next read.
//
// # Boundary: the console never writes providers.yaml
//
// Not "not yet" — not at all. Two reasons, and the second is the one that keeps
// it true:
//
//  1. base_url is the SSRF entrance. A base_url editable from a web page means a
//     compromised console can point the backend at 169.254.169.254 and read the
//     result back out through a weather forecast the model then repeats to the
//     user. Requiring shell access to change it is the whole defence.
//  2. Safely rewriting a YAML file that an operator also hand-edits is a genuinely
//     hard problem — comments are lost, ordering is lost, concurrent edits
//     clobber, and there is a read-to-write gap. Not doing it means not having
//     any of that, and the price is only "changing base_url means logging in",
//     which was always going to be true.
//
// The same rule answers models.yaml and oauth.yaml in F4b: the console writes
// their runtime overrides, never the files.
type ProviderOverride struct {
	// Kind is "weather" | "search" | "channel". Part of the key because ids are
	// only unique within a capability — two different sources may both sensibly
	// be called "primary".
	Kind string `json:"kind"`
	// ID matches an entry in providers.yaml. An override for an id that is no
	// longer in the file is kept, not deleted: files get moved between machines
	// and a half-migrated deployment should not silently lose the operator's
	// settings. It is reported as orphaned instead.
	ID string `json:"id"`

	// Enabled is a pointer so "no opinion" and "explicitly off" are different.
	// Without that, an override row could never mean "fall back to the file",
	// and the only way to undo a console change would be to delete the row —
	// which the console cannot express in a form that survives a re-save.
	Enabled *bool `json:"enabled,omitempty"`

	// Description is the operator's text, and the ONLY text that may reach a
	// prompt. Per locale; both must be present for it to be usable, the same
	// double-locale rule the prompt templates are held to.
	Description map[string]string `json:"description,omitempty"`

	// DescriptionHash is what Approved was granted against.
	//
	// Approval is "I read this text", so it cannot outlive the text. Storing the
	// hash rather than a boolean alone is what makes editing a description
	// revoke its approval automatically — otherwise the gate is one edit away
	// from being decorative, and nothing would go red.
	DescriptionHash string `json:"descriptionHash,omitempty"`
	// Approved gates Description into the system prompt. Never inferred, never
	// defaulted true: an unapproved source still works, the backend just
	// describes it mechanically from its id and capabilities.
	Approved bool `json:"approved"`

	UpdatedAt time.Time `json:"updatedAt"`
}

// ProviderOverrideRepository stores the console-editable half.
//
// Deployment-wide, not per session: these are the operator's settings and there
// is one operator view — the same shape as Setting, and deliberately unlike
// almost every other repository here, which is why it is worth saying.
type ProviderOverrideRepository interface {
	// All returns every override. A small table read at boot and after each
	// change, so there is no filter to keep consistent across four backends.
	All(ctx context.Context) ([]ProviderOverride, error)
	// Set upserts one override, keyed by (Kind, ID).
	Set(ctx context.Context, o ProviderOverride) error
	// Delete removes an override, restoring whatever the file says. Deleting a
	// row that is not there is not an error — the console cannot know whether
	// one exists, and making it find out first would be a race.
	Delete(ctx context.Context, kind, id string) error
}
