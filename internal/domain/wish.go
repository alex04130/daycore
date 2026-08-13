package domain

import "time"

// Wish statuses. The pool only has two states that matter to anything reading
// it: still wanted, or done with.
//
// ⚠️ Constants because the daemon's gap-filler is the first READER of this
// field — until it existed the value was written by one tool and never
// compared, so a typo in the literal would have cost nothing. It costs
// something now: a wish written as "Active" would be permanently invisible to
// the thing that exists to surface it.
const (
	WishActive = "active"
	WishDone   = "done"
)

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
