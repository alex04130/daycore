package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordInShapes(t *testing.T) {
	cases := []struct{ dsn, want string }{
		{"postgres://user:secret@host:5432/db", "secret"},
		{"mysql://user:secret@tcp(host:3306)/db", "secret"},
		{"mongodb://user:secret@host:27017/db", "secret"},
		{"mongodb+srv://user:secret@cluster.example.net/db", "secret"},
		{"file:daycore.db", ""},
		{"postgres://user@host/db", ""},
		{"postgres://user:@host/db", ""},
	}
	for _, tc := range cases {
		if got := passwordIn(tc.dsn); got != tc.want {
			t.Errorf("passwordIn(%q) = %q, want %q", tc.dsn, got, tc.want)
		}
	}
}

func TestRedactDSN(t *testing.T) {
	// The password never survives into the error the terminal shows.
	dsn := "postgres://user:supersecret@host:5432/db"
	err := errors.New("dial postgres://user:supersecret@host:5432/db: connection refused")
	got := redactDSN(err, dsn).Error()
	if strings.Contains(got, "supersecret") {
		t.Errorf("the password leaked: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Errorf("the password must be replaced by a marker: %q", got)
	}
	// A nil error passes through as nil.
	if redactDSN(nil, dsn) != nil {
		t.Error("a nil error must stay nil")
	}
}
