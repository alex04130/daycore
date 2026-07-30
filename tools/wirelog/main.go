// Command wirelog is a logging reverse proxy for inspecting what Daycore
// actually sends to a model provider.
//
// TLS makes tcpdump useless here — the interesting bytes are inside the
// encrypted stream. But models.yaml gives every model its own base_url, so the
// client can be pointed at this instead, which sees the plaintext on both sides
// and then speaks TLS upstream itself. No MITM certificate, no /etc/hosts edit.
//
//	go run . -listen 127.0.0.1:8899 -upstream https://api.deepseek.com -out wire.log
//
// The response body is teed rather than buffered: an SSE stream has to keep
// flowing to the client while it is being recorded, and buffering it would turn
// the streaming path — the one actually under test — into a non-streaming one.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	listen   = flag.String("listen", "127.0.0.1:8899", "address to listen on")
	upstream = flag.String("upstream", "", "upstream base URL, e.g. https://api.deepseek.com")
	out      = flag.String("out", "wire.log", "file to append the transcript to")
)

var seq atomic.Int64

func main() {
	flag.Parse()
	if *upstream == "" {
		log.Fatal("-upstream is required")
	}
	up, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("bad -upstream: %v", err)
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		log.Fatalf("mkdir -out: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}

	srv := &http.Server{
		Addr: *listen,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			n := seq.Add(1)
			// One file per exchange. A single shared file cannot survive
			// concurrency: fragments from overlapping requests interleave and the
			// transcript stops being parseable — which is what happened the first
			// time this ran while a browser was also pointed at the proxy.
			f, ferr := os.Create(fmt.Sprintf("%s/%04d-%s.txt", *out, n, sanitize(r.URL.Path)))
			if ferr != nil {
				http.Error(rw, ferr.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			w := &syncWriter{f: f}

			body, _ := io.ReadAll(r.Body)
			r.Body.Close()

			target := up.JoinPath(r.URL.Path)
			target.RawQuery = r.URL.RawQuery

			w.logf("\n═══ #%d %s %s → %s  %s\n", n, r.Method, r.URL.Path, target.String(),
				time.Now().Format("15:04:05.000"))
			w.logf("── request headers\n")
			for k, vs := range r.Header {
				for _, v := range vs {
					// The key is the one thing that must not land in a log file
					// that gets read out loud.
					if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "X-Api-Key") {
						v = "<redacted>"
					}
					w.logf("%s: %s\n", k, v)
				}
			}
			w.logf("── request body (%d bytes)\n%s\n", len(body), pretty(body))

			req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(body))
			if err != nil {
				http.Error(rw, err.Error(), http.StatusBadGateway)
				return
			}
			req.Header = r.Header.Clone()
			req.Host = up.Host
			req.Header.Del("Accept-Encoding") // keep the recorded body readable

			resp, err := client.Do(req)
			if err != nil {
				w.logf("── UPSTREAM ERROR: %v\n", err)
				http.Error(rw, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()

			w.logf("── response %s  (content-type %s)\n", resp.Status, resp.Header.Get("Content-Type"))
			for k, vs := range resp.Header {
				rw.Header()[k] = vs
			}
			rw.WriteHeader(resp.StatusCode)

			// Tee while streaming. Flush after every chunk or the SSE frames sit
			// in a buffer and the client sees one blob at the end — which would
			// hide exactly the incremental behaviour being tested.
			w.logf("── response body\n")
			buf := make([]byte, 8192)
			for {
				k, rerr := resp.Body.Read(buf)
				if k > 0 {
					w.write(buf[:k])
					if _, werr := rw.Write(buf[:k]); werr != nil {
						w.logf("\n── CLIENT WENT AWAY: %v\n", werr)
						return
					}
					if fl, ok := rw.(http.Flusher); ok {
						fl.Flush()
					}
				}
				if rerr != nil {
					if rerr != io.EOF {
						w.logf("\n── UPSTREAM READ ERROR: %v\n", rerr)
					}
					break
				}
			}
			w.logf("\n═══ #%d end\n", n)
		}),
	}
	log.Printf("wirelog: %s → %s, one file per exchange in %s/", *listen, *upstream, *out)
	log.Fatal(srv.ListenAndServe())
}

type syncWriter struct {
	mu sync.Mutex
	f  *os.File
}

func (w *syncWriter) logf(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintf(w.f, format, args...)
}

func (w *syncWriter) write(b []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.f.Write(b)
}

// pretty re-indents JSON so a tools array is readable, and leaves anything else
// untouched.
func pretty(b []byte) string {
	var buf bytes.Buffer
	if err := indentJSON(&buf, b); err != nil {
		return string(b)
	}
	return buf.String()
}

func indentJSON(dst *bytes.Buffer, src []byte) error {
	return json.Indent(dst, src, "", "  ")
}

// sanitize turns a URL path into a filename fragment.
func sanitize(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "root"
	}
	return strings.NewReplacer("/", "_", "?", "_", ":", "_", " ", "_").Replace(p)
}
