package domain

import (
	"context"
	"time"
)

type TempContext struct {
	ID        string
	SessionID string
	Key       string
	Payload   string
	TTL       time.Time
	CreatedAt time.Time
}

type TempContextRepository interface {
	Get(ctx context.Context, sid, key string) (*TempContext, error)
	Set(ctx context.Context, tc *TempContext) error
	Delete(ctx context.Context, sid, key string) error
	Expire(ctx context.Context) (int, error)
}
