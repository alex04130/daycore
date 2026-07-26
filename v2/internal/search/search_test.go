package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const ddgPage = `<html><body>
<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fkmp&amp;rut=abc">KMP <b>算法</b>详解</a>
<a class="result__snippet" href="#">字符串匹配的<b>经典</b>算法…</a>
<a class="result__a" href="https://duckduckgo.com/y.js?ad=1">广告位</a>
<a class="result__a" href="https://plain.example.org/x">Plain link</a>
<div class="result__snippet">second snippet</div>
</body></html>`

func TestTavilyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"title":"A","url":"https://a.com","content":"aaa"},
			{"title":"B","url":"https://b.com","content":"bbb"},
			{"title":"C","url":"https://c.com","content":"ccc"}]}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), TavilyKey: "k", TavilyURL: srv.URL, DDGURL: "http://127.0.0.1:1"}

	rs, err := c.Search(context.Background(), "kmp", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Title != "A" || rs[1].Snippet != "bbb" {
		t.Fatalf("results = %+v", rs)
	}
}

func TestTavilyErrorFallsBackToDDG(t *testing.T) {
	tav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tav.Close()
	ddg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ddgPage))
	}))
	defer ddg.Close()
	c := &Client{HTTP: http.DefaultClient, TavilyKey: "k", TavilyURL: tav.URL, DDGURL: ddg.URL}

	rs, err := c.Search(context.Background(), "kmp", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("results = %+v", rs)
	}
	if rs[0].URL != "https://example.com/kmp" {
		t.Errorf("uddg redirect not decoded: %q", rs[0].URL)
	}
	if rs[0].Title != "KMP 算法详解" {
		t.Errorf("tags not stripped: %q", rs[0].Title)
	}
	if rs[1].URL != "https://plain.example.org/x" {
		t.Errorf("ad slot not skipped / plain href broken: %+v", rs[1])
	}
}

func TestNoKeyGoesStraightToDDG(t *testing.T) {
	tavHit := false
	tav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tavHit = true
	}))
	defer tav.Close()
	ddg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ddgPage))
	}))
	defer ddg.Close()
	c := &Client{HTTP: http.DefaultClient, TavilyKey: "", TavilyURL: tav.URL, DDGURL: ddg.URL}

	rs, err := c.Search(context.Background(), "kmp", 1)
	if err != nil {
		t.Fatal(err)
	}
	if tavHit {
		t.Error("tavily was called without a key")
	}
	if len(rs) != 1 {
		t.Fatalf("results = %+v", rs)
	}
}

func TestParseDDGLenient(t *testing.T) {
	if got := parseDDG("<html>totally different layout</html>", 3); len(got) != 0 {
		t.Errorf("expected empty slice on layout drift, got %+v", got)
	}
}
