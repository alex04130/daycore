// Package storage is the driver registry that turns a DB_TYPE string into a
// domain.Store. Concrete adapters (sqlstore, mongostore, …) self-register via
// Register in their init(); main.go imports them for side effects. Adding a new
// database = implement domain.Store in a package + Register + recompile.
package storage

import (
	"fmt"
	"sort"
	"sync"

	"daycore/internal/domain"
)

// Opener constructs a Store from a DSN.
type Opener func(dsn string) (domain.Store, error)

var (
	mu      sync.RWMutex
	openers = map[string]Opener{}
)

// Register adds a database driver under dbType. Panics on duplicate/empty (programmer error).
func Register(dbType string, o Opener) {
	mu.Lock()
	defer mu.Unlock()
	if dbType == "" || o == nil {
		panic("storage: invalid Register arguments")
	}
	if _, dup := openers[dbType]; dup {
		panic("storage: db type already registered: " + dbType)
	}
	openers[dbType] = o
}

// Open builds a Store for dbType using dsn.
func Open(dbType, dsn string) (domain.Store, error) {
	mu.RLock()
	o, ok := openers[dbType]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", domain.ErrUnsupportedDBType, dbType, Types())
	}
	return o(dsn)
}

// Types lists registered database types (sorted).
func Types() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(openers))
	for t := range openers {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
