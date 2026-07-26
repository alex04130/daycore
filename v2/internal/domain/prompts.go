// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// Prompt is a runtime-editable override of a built-in prompt template, keyed by
// (Key, Locale) so every locale's template can be tuned independently.
type Prompt struct {
	Key       string    `json:"key"`
	Locale    string    `json:"locale"` // e.g. "zh-CN" | "en-US"
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updatedAt"`
}
