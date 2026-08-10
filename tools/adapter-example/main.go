// Command adapter-example is a runnable reference implementation of the Daycore
// external capability protocol (docs/specs/provider-protocol.md).
//
// # Why it is in this repository
//
// Two jobs, and it does the second one because of the first.
//
//  1. **It is the example a third party copies.** The protocol document
//     describes the frames; this is a program that answers them. Anybody adding
//     a weather or search source can read one file and see the whole shape,
//     including the parts a prose specification says badly — what an error body
//     looks like, what an empty result means, where the token check goes.
//
//  2. **It is the only honest test of `format: http`.** Standing up an
//     httptest server inside the client's own package and pointing the client at
//     it proves that our encoder agrees with our decoder — it is the
//     specification transcribed twice and compared, which is exactly as strong
//     as reading it twice. This is a separate program, with its own parsing,
//     started as a real subprocess over a real socket. When
//     TestReferenceAdapterSpeaksTheProtocol passes, a third-party adapter
//     written from the same document will work.
//
// # Boundary: it is a reference, not a product
//
// It serves canned data. It is not a weather source and must never grow into
// one — the moment it needs an upstream API key it stops being runnable by
// somebody reading the docs, which is the whole point of it.
//
// Usage:
//
//	go run ./tools/adapter-example -addr 127.0.0.1:8099 -token secret
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address; :0 picks a free port and prints it")
	token := flag.String("token", "", "if set, requests must carry it as a bearer token")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	// The port goes to stdout on one line so a supervising process can read it
	// without guessing. Everything else this program says goes to stderr —
	// the same split docs/specs/transport.md requires of an `exec` adapter, kept
	// here so the two shapes read alike.
	fmt.Printf("http://%s\n", ln.Addr().String())
	os.Stdout.Sync()

	mux := http.NewServeMux()
	mux.HandleFunc("/v0/manifest", guard(*token, manifest))
	mux.HandleFunc("/v0/weather", guard(*token, weather))
	mux.HandleFunc("/v0/search", guard(*token, search))

	srv := &http.Server{Handler: logging(mux), ReadHeaderTimeout: 5 * time.Second}
	log.SetOutput(os.Stderr)
	log.Printf("adapter-example listening on %s (token required: %v)", ln.Addr(), *token != "")
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// guard enforces the bearer token.
//
// The specification says an adapter MUST check the token when one is
// configured, and 401 is mapped by the backend to "stop calling this source"
// rather than "retry" — so getting this wrong makes an adapter look
// intermittently broken instead of misconfigured.
func guard(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			fail(w, http.StatusUnauthorized, "bad or missing bearer token")
			return
		}
		next(w, r)
	}
}

// logging echoes the correlation id. Cross-process debugging has nothing else
// to join on, which is why the backend sends it on every call.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s id=%s deadline=%sms", r.Method, r.URL.Path,
			r.Header.Get("X-Daycore-Request-Id"), r.Header.Get("X-Daycore-Deadline-Ms"))
		next.ServeHTTP(w, r)
	})
}

// ── /v0/manifest ────────────────────────────────────────────────────────────

func manifest(w http.ResponseWriter, r *http.Request) {
	// A 1×1 PNG. The logo must be a raster data: URI — the backend refuses SVG,
	// because an SVG can carry script and a channel's logo renders in ordinary
	// user clients, not just the admin console.
	png := []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0,
		0x1f, 0x15, 0xc4, 0x89,
	}
	writeJSON(w, map[string]any{
		"name":         "adapter-example",
		"displayName":  "Reference Adapter",
		"logo":         "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"type":         "query",
		"protocol":     1,
		"capabilities": []string{"search", "weather"},
		// ⚠️ This is a SUGGESTION and the backend treats it as one. It is never
		// injected into a prompt: an operator has to copy it into providers.yaml
		// or the console and approve it, which records the approval against a
		// hash of the exact text. Writing instructions here does nothing.
		"description": map[string]string{
			"zh-CN": "参考适配层，返回固定数据，用来验证协议本身。",
			"en-US": "Reference adapter. Returns canned data; exists to exercise the protocol.",
		},
	})
}

// ── /v0/weather ─────────────────────────────────────────────────────────────

type weatherReq struct {
	Location string `json:"location"`
	Days     int    `json:"days"`
	Locale   string `json:"locale"`
}

func weather(w http.ResponseWriter, r *http.Request) {
	var req weatherReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Location) == "" {
		// 400 means "the request itself is wrong". The backend does NOT retry
		// it — retrying a malformed request just produces three of them.
		fail(w, http.StatusBadRequest, "location is required")
		return
	}
	days := req.Days
	if days < 1 {
		days = 2
	}
	if days > 7 {
		days = 7
	}

	// ⚠️ An empty days array is treated by the backend as a FAILURE, not as an
	// empty forecast — an empty forecast renders as an ordinary-looking sentence
	// with the weather quietly missing, and it would count as a successful call,
	// so a source answering 200-with-nothing forever would stay marked healthy.
	// Say what you know, or return an error.
	out := make([]map[string]any, 0, days)
	base := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	for i := 0; i < days; i++ {
		out = append(out, map[string]any{
			"date": base.AddDate(0, 0, i).Format("2006-01-02"),
			// WMO code, or -1 when this source has none. 0 is a real code (clear
			// sky), which is why absent and zero have to be different.
			"code": 3,
			// Localised BY THE ADAPTER, from the requested locale: only the
			// adapter knows which languages its upstream speaks. Fall back to
			// your own default rather than failing — a forecast in the wrong
			// language still says it is going to rain.
			"text":       localized(req.Locale, "阴", "Overcast"),
			"tempMin":    21 + float64(i),
			"tempMax":    29 + float64(i),
			"precipProb": 40,
		})
	}
	writeJSON(w, map[string]any{"location": req.Location, "days": out})
}

func localized(locale, zh, en string) string {
	if strings.HasPrefix(locale, "zh") {
		return zh
	}
	return en
}

// ── /v0/search ──────────────────────────────────────────────────────────────

type searchReq struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
	Locale string `json:"locale"`
}

func search(w http.ResponseWriter, r *http.Request) {
	var req searchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Query) == "" {
		fail(w, http.StatusBadRequest, "query is required")
		return
	}
	limit := req.Limit
	if limit < 1 || limit > 5 {
		limit = 3
	}
	// Unlike weather, an EMPTY result list here is a legitimate answer and the
	// backend accepts it: "nothing matched" is true and useful, and treating it
	// as a failure would mark a working engine unhealthy for being asked about
	// something obscure.
	results := make([]map[string]string, 0, limit)
	for i := 0; i < limit; i++ {
		results = append(results, map[string]string{
			"title":   fmt.Sprintf("%s — result %d", req.Query, i+1),
			"url":     fmt.Sprintf("https://example.invalid/%d", i+1),
			"snippet": "Canned text from the reference adapter.",
		})
	}
	writeJSON(w, map[string]any{"results": results})
}

// ── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// fail writes an error the backend can classify.
//
// The status code is the contract; the body is for a human reading a log. The
// backend never shows this text to a model or a user — an adapter's own words
// routinely quote the URL it was called on, and that can carry an internal
// hostname or a key.
func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
