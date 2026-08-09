package setup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PingDSN opens a connection, says hello, and closes it.
//
// # What it is and is not
//
// It answers exactly one question: can this process reach that database with
// those credentials. It does not create the database, does not run migrations,
// and does not look at a single table. Migrations belong to the server's boot
// path, where a failure has somewhere to be reported and something to report it
// to — running them from an installer means a half-migrated schema whose only
// witness is a terminal the operator has already closed.
//
// # Boundary: never fatal
//
// Every caller treats a failure as a warning. The check exists because a
// mistyped DSN is the most common setup mistake, not because the database has
// to exist yet — on a fresh box the usual order is "install the app, then
// provision the database", and an installer that refuses to finish until the
// database answers cannot be used in that order.
//
// The five-second budget is the whole call, not per attempt: this runs in front
// of somebody waiting at a prompt, and a check that hangs is worse than no
// check, because they cannot tell it apart from a crash.
func PingDSN(engine, dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch engine {
	case "sqlite":
		return nil // a file that does not exist yet is not a failure
	case "mongodb":
		cli, err := mongo.Connect(ctx, options.Client().ApplyURI(dsn).SetServerSelectionTimeout(5*time.Second))
		if err != nil {
			return redactDSN(err, dsn)
		}
		defer cli.Disconnect(context.Background())
		return redactDSN(cli.Ping(ctx, nil), dsn)
	case "postgres", "mysql":
		driver := map[string]string{"postgres": "pgx", "mysql": "mysql"}[engine]
		db, err := sql.Open(driver, dsn)
		if err != nil {
			return redactDSN(err, dsn)
		}
		defer db.Close()
		return redactDSN(db.PingContext(ctx), dsn)
	}
	return fmt.Errorf("unknown engine %q", engine)
}

// redactDSN keeps the password out of the message.
//
// Driver errors routinely quote the DSN back, and the DSN routinely contains a
// password. This is printed to a terminal, and terminals get screenshotted into
// issue reports — the same reasoning that keeps the degraded-boot reason behind
// an authenticated path.
func redactDSN(err error, dsn string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if pw := passwordIn(dsn); pw != "" {
		msg = strings.ReplaceAll(msg, pw, "***")
	}
	return errors.New(msg)
}

// passwordIn pulls the secret out of the three DSN shapes this project accepts,
// so it can be redacted. Returns "" when there is nothing to hide.
func passwordIn(dsn string) string {
	// scheme://user:pass@host…
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := strings.LastIndex(rest, "@"); at > 0 {
			if c := strings.Index(rest[:at], ":"); c >= 0 {
				return rest[c+1 : at]
			}
		}
		return ""
	}
	// user:pass@tcp(host:port)/db
	if at := strings.LastIndex(dsn, "@"); at > 0 {
		if c := strings.Index(dsn[:at], ":"); c >= 0 {
			return dsn[c+1 : at]
		}
	}
	return ""
}
