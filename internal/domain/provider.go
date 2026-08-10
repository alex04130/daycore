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
//	config/providers.yaml   id, format, token_env, capabilities   the file
//	provider_overrides      enabled, base_url, description, approved
//
// # Boundary: the console never writes providers.yaml
//
// Not "not yet" — not at all, and the reason is NOT about which fields are
// sensitive. It is that safely rewriting a YAML file an operator also hand-edits
// is a genuinely hard problem: comments are lost, ordering is lost, concurrent
// edits clobber each other, and there is a read-to-write gap. Not doing it means
// not having any of that.
//
// So a field the console must be able to change lives in the TABLE, and the
// table wins over the file. base_url moved here on 2026-08-09 for exactly that
// reason — see the field comment for what that costs and what still holds.
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

	// BaseURL overrides where an http adapter lives. Empty means "use the file".
	//
	// # It used to be file-only, and that reasoning did not hold up
	//
	// The argument was "base_url is the SSRF entrance, so changing it should
	// require shell access". But the same batch that wrote it also concluded, in
	// internal/adapters/baseurl.go, that **an attacker who can edit
	// providers.yaml already has the machine** — which makes "who may set it" a
	// weak defence in both directions.
	//
	// What actually holds the SSRF line is independent of who set the value:
	// link-local and credential-bearing URLs are refused by validateBaseURL, and
	// the HTTP client refuses to follow redirects. Those two apply to a value
	// typed into a console exactly as they apply to one read from a file.
	//
	// ⚠️ What IS genuinely lost, so nobody thinks it was free: reaching this
	// field used to need shell access, and now it needs a console credential —
	// which can be phished or stolen through the browser in ways a shell cannot.
	// The mitigation is that the value is still validated on the way in, and
	// that changing it is visible (it is an override row with a timestamp).
	BaseURL string `json:"baseUrl,omitempty"`

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
