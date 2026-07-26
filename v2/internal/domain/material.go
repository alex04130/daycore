package domain

import "time"

type Material struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Category   string    `json:"category"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	Body       string    `json:"body"`
	Source     string    `json:"source"`
	MimeType   string    `json:"mime_type"`
	StorageRef string    `json:"storage_ref"`
	Tags       []string  `json:"tags"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type MaterialRepository interface {
	List(ctx interface{}, sessionID, category, query string, limit, offset int) ([]Material, error)
	Get(ctx interface{}, sessionID, id string) (*Material, error)
	Create(ctx interface{}, material *Material) (*Material, error)
	Update(ctx interface{}, sessionID, id string, material *Material) (*Material, error)
	Delete(ctx interface{}, sessionID, id string) error
}
