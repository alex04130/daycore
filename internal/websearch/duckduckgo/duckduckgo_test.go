package duckduckgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// parseDDG is deliberately lenient: layout drift yields an empty slice,
// never an error — but a page with the real shape must parse every field.
func TestParseDDG(t *testing.T) {
	page := `<div class="result">` +
		`<a class="result__a" href="https://example.com/a">First <b>Result</b></a>` +
		`<div class="result__snippet">Snippet <em>one</em></div>` +
		`<a class="result__a" href="//example.com/b">Second</a>` +
		`<div class="result__snippet">Snippet two</div>` +
		`<a class="result__a" href="https://duckduckgo.com/y.js?u=x">Ad</a>` +
		`<a class="result__a" href=""></a>` +
		`<a class="result__a" href="https://example.com/c">Third</a>`
	got := parseDDG(page, 10)
	// Ad slot and the empty anchor are skipped: First, Second, Third.
	if len(got) != 3 {
		t.Fatalf("parseDDG = %d results, want 3: %+v", len(got), got)
	}
	if got[0].Title != "First Result" || got[0].URL != "https://example.com/a" || got[0].Snippet != "Snippet one" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].URL != "https://example.com/b" {
		t.Errorf("a protocol-relative href must gain https:, got %+v", got[1])
	}
	// maxResults is a hard cap.
	if got := parseDDG(page, 2); len(got) != 2 {
		t.Errorf("maxResults must cap, got %d", len(got))
	}
}

func TestParseDDGGarbageYieldsEmpty(t *testing.T) {
	for _, page := range []string{"", "<html>no results here</html>", "not even html"} {
		if got := parseDDG(page, 5); len(got) != 0 {
			t.Errorf("parseDDG(garbage) = %+v, want empty", got)
		}
	}
}

func TestDecodeHref(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/x", "https://example.com/x"},
		{"/l/?uddg=https%3A%2F%2Fexample.com%2Fx&amp;rut=abc", "https://example.com/x"},
		{"//example.com/x", "https://example.com/x"},
		{"/l/?uddg=", "/l/?uddg="},
	}
	for _, tc := range cases {
		if got := decodeHref(tc.in); got != tc.want {
			t.Errorf("decodeHref(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSearchHTTPErrorAndEmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	e := &engine{url: srv.URL, http: srv.Client()}
	if _, err := e.Search(context.Background(), "q", 5); err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("an HTTP error must be reported, got %v", err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html></html>"))
	}))
	defer srv2.Close()
	e2 := &engine{url: srv2.URL, http: srv2.Client()}
	// Zero results is a valid answer, not an error.
	if got, err := e2.Search(context.Background(), "q", 5); err != nil || len(got) != 0 {
		t.Errorf("empty results must be (empty, nil), got (%v, %v)", got, err)
	}
}
