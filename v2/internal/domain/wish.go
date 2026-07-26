package domain

import "time"

type Wish struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Title     string    `json:"title"`
	Note      string    `json:"note"`
	EffortMin int       `json:"effortMin"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WishRepository interface {
	List(ctx interface{}, sid, status string) ([]Wish, error)
	Get(ctx interface{}, sid, id string) (*Wish, error)
	Create(ctx interface{}, w *Wish) (*Wish, error)
	Update(ctx interface{}, sid, id string, w *Wish) (*Wish, error)
	Delete(ctx interface{}, sid, id string) error
}
