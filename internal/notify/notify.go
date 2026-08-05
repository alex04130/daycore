// Package notify is the delivery bus for the fact track: notifications that
// must reach the user even when no client is connected (a deadline reminder,
// the Protector nudge). Suggestions never travel this bus — the attention
// ladder keeps them in-app.
//
// The registry mirrors internal/storage and internal/blob: a driver name
// selects an implementation, drivers self-register from init(), and main.go
// imports them for side effects. Web Push, APNs, FCM and channel outbound are
// all legitimate drivers; a chat-platform channel is just the first one the
// product happened to grow.
//
// nil is a supported configuration. With no driver registered, fact-track
// delivery degrades to whatever channels are configured — which today means
// "nothing outside the app", a known gap tracked in the roadmap. Features that
// need guaranteed delivery must check and say so rather than fail obscurely.
//
// What a Notifier is not: it is not targeting or scheduling. When to notify
// and whom is the caller's business (Worker, proposals); a driver only knows
// how to reach one session's devices.
package notify

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Notification is one fact-track message to one session.
type Notification struct {
	SessionID string
	Title     string
	Body      string
	// URL is where a tap lands. Empty means "just open the app".
	URL string
}

// Notifier delivers notifications to a session's devices.
type Notifier interface {
	Send(ctx context.Context, n Notification) error
}

// Opener builds a driver from its configuration. The config shape is the
// driver's own — a directory, a VAPID keypair, a webhook URL.
type Opener func(cfg map[string]any) (Notifier, error)

var (
	mu      sync.RWMutex
	drivers = map[string]Opener{}
)

// Register adds a driver. Called from driver packages' init(), like the
// storage and blob registries; registering the same name twice panics, because
// two drivers silently disagreeing about a name is worse than a crash at boot.
func Register(name string, o Opener) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" || o == nil {
		panic("notify: invalid Register arguments")
	}
	if _, dup := drivers[name]; dup {
		panic("notify: driver registered twice: " + name)
	}
	drivers[name] = o
}

// Open builds the named driver. An empty name is the supported "no push
// configured" case and returns (nil, nil) — callers must tolerate a nil
// Notifier.
func Open(name string, cfg map[string]any) (Notifier, error) {
	if name == "" {
		return nil, nil
	}
	mu.RLock()
	o, ok := drivers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("notify: unsupported driver %q (registered: %v)", name, Drivers())
	}
	return o(cfg)
}

// Drivers lists the registered driver names, for config validation and the
// admin console.
func Drivers() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(drivers))
	for name := range drivers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
