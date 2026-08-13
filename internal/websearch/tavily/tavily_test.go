package tavily

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tavilyEngine(t *testing.T, handler http.HandlerFunc) *engine {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &engine{url: srv.URL, key: "k", http: srv.Client()}
}

func TestTavilyParseAndClamp(t *testing.T) {
	e := tavilyEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[
			{"title":"一","url":"https://a","content":"c1"},
			{"title":"二","url":"https://b","content":"c2"},
			{"title":"三","url":"https://c","content":"c3"}
		]}`))
	})
	got, err := e.Search(context.Background(), "q", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "一" || got[1].Snippet != "c2" {
		t.Errorf("maxResults must cap the parsed list, got %+v", got)
	}
}

func TestTavilyErrors(t *testing.T) {
	e := tavilyEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := e.Search(context.Background(), "q", 3); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("an HTTP error must be reported, got %v", err)
	}
	e2 := tavilyEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})
	if _, err := e2.Search(context.Background(), "q", 3); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("a bad body must be reported as a decode error, got %v", err)
	}
}
