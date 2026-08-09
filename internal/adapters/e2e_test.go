package adapters_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"daycore/internal/adapters"
	"daycore/internal/domain"
	"daycore/internal/weather"
	"daycore/internal/websearch"

	_ "daycore/internal/weather/openmeteo"
	_ "daycore/internal/websearch/duckduckgo"
)

// The end-to-end check for `format: http`.
//
// # Why a subprocess and not httptest
//
// An httptest server inside this package, answered by handlers written next to
// the client that calls them, proves that our encoder agrees with our decoder.
// That is the specification transcribed twice and compared — exactly as strong
// as reading it twice, and it would pass just as happily if both halves were
// wrong in the same way.
//
// tools/adapter-example is a separate program with its own parsing, started
// here as a real process over a real socket. It is also the file a third party
// copies. So when this passes, what has been demonstrated is that an
// independently written adapter works — which is the claim `format: http` makes
// and the only one worth testing.
//
// It costs a `go run` (a few seconds, once) and skips in -short.
// adapterBinary compiles the reference adapter once for the whole package.
//
// Compiled and exec'd DIRECTLY rather than run through `go run`. That is not an
// optimisation: `go run` builds a binary and runs it as its OWN child, so
// killing the `go run` process leaves the server alive holding the stderr pipe,
// and the test binary then sits for a full minute waiting on I/O that will
// never end. The first version of this file did exactly that — every run passed
// and then hung for 60 seconds.
var adapterBinary = sync.OnceValues(func() (string, error) {
	root, err := filepath.Abs("../..")
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "daycore-adapter")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "adapter-example")
	cmd := exec.Command("go", "build", "-o", bin, "./tools/adapter-example")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build reference adapter: %v\n%s", err, out)
	}
	return bin, nil
})

func startReferenceAdapter(t *testing.T, token string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: the reference adapter needs a compile")
	}
	bin, err := adapterBinary()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-addr", "127.0.0.1:0"}
	if token != "" {
		args = append(args, "-token", token)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start adapter: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// The address arrives on the first line of stdout — the same handshake shape
	// docs/specs/transport.md defines for an `exec` adapter, so the two forms
	// read alike.
	line := make(chan string, 1)
	go func() {
		r := bufio.NewReader(stdout)
		s, _ := r.ReadString('\n')
		line <- strings.TrimSpace(s)
	}()
	select {
	case addr := <-line:
		if !strings.HasPrefix(addr, "http://") {
			t.Fatalf("adapter announced %q, want a URL", addr)
		}
		return addr
	case <-time.After(10 * time.Second):
		t.Fatal("the adapter never announced an address")
	}
	return ""
}

// The whole path: a providers.yaml naming an http source, resolved into a set,
// answering a real lookup over a socket.
func TestReferenceAdapterSpeaksTheWeatherProtocol(t *testing.T) {
	base := startReferenceAdapter(t, "")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(cfg, []byte(
		"weather:\n  - id: reference\n    format: http\n    base_url: "+base+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	file, err := adapters.Load(cfg)
	if err != nil {
		t.Fatalf("the config the example needs does not even load: %v", err)
	}
	entries := file.Entries(adapters.KindWeather)
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	src := adapters.Resolve(adapters.KindWeather, entries[0], nil, adapters.NewHealth())
	set, problems := weather.NewSources([]*adapters.Source{src}, weather.Options{})
	if len(problems) > 0 {
		t.Fatalf("building the source: %v", problems)
	}

	fc, err := set.Lookup(context.Background(), "reference",
		domain.WeatherQuery{Location: "上海", Days: 3, Locale: "zh-CN"})
	if err != nil {
		t.Fatalf("lookup through a real adapter failed: %v", err)
	}
	if fc.Location != "上海" {
		t.Errorf("location = %q", fc.Location)
	}
	if len(fc.Days) != 3 {
		t.Fatalf("got %d days, asked for 3", len(fc.Days))
	}
	if fc.Days[0].Text == "" || fc.Days[0].TempMax == 0 {
		t.Errorf("a day came back empty: %+v", fc.Days[0])
	}
	// The adapter localises its own text, because only it knows which languages
	// its upstream speaks.
	if fc.Days[0].Text != "阴" {
		t.Errorf("text = %q; the adapter was asked for zh-CN", fc.Days[0].Text)
	}
	// A real call was observed, so the health machine has something to work with.
	if snap := src.Health.Snapshot(); snap.LastChecked.IsZero() {
		t.Error("a real call was not recorded as a health observation")
	}

	// The manifest, which is what the console renders. The logo is a raster
	// data: URI or nothing — SVG is refused because it can carry script.
	m, err := adapters.FetchManifest(context.Background(),
		adapters.NewClient("reference", base, "", 10*time.Second))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if m.DisplayName == "" || !strings.HasPrefix(m.Logo, "data:image/png;base64,") {
		t.Errorf("manifest did not survive validation: %+v", m)
	}
	if len(m.Description) != 2 {
		t.Errorf("the example's description is not in both locales: %v", m.Description)
	}

	// And the gate: what the adapter said about itself does not reach a prompt.
	src.SetManifest(m)
	if got := src.PromptDescription("zh-CN"); strings.Contains(got, "参考适配层") {
		t.Errorf("the adapter's own description reached the prompt: %q", got)
	}
}

func TestReferenceAdapterSpeaksTheSearchProtocol(t *testing.T) {
	base := startReferenceAdapter(t, "")
	src := adapters.Resolve(adapters.KindSearch,
		adapters.Entry{ID: "reference", Format: adapters.FormatHTTP, BaseURL: base}, nil, adapters.NewHealth())
	set, problems := websearch.NewSources([]*adapters.Source{src}, websearch.Options{})
	if len(problems) > 0 {
		t.Fatalf("building the source: %v", problems)
	}

	rs, err := set.Search(context.Background(), "reference", "石化线是什么", 2)
	if err != nil {
		t.Fatalf("search through a real adapter failed: %v", err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d results, asked for 2", len(rs))
	}
	if rs[0].Title == "" || rs[0].URL == "" {
		t.Errorf("a result came back empty: %+v", rs[0])
	}
}

// The token is checked by the adapter and 401 is not retried — a wrong token
// does not become right, and treating it as transient makes a misconfigured
// source look intermittently broken.
func TestBearerTokenIsEnforcedEndToEnd(t *testing.T) {
	base := startReferenceAdapter(t, "s3cret")

	bad := adapters.NewClient("reference", base, "wrong", 10*time.Second)
	_, err := adapters.FetchManifest(context.Background(), bad)
	e, ok := adapters.AsError(err)
	if !ok {
		t.Fatalf("got %v, want an adapters.Error", err)
	}
	if e.Kind != adapters.KindAuth {
		t.Errorf("kind = %s, want auth", e.Kind)
	}
	if e.Retryable() {
		t.Error("an auth failure is retryable; a wrong token does not become right")
	}

	good := adapters.NewClient("reference", base, "s3cret", 10*time.Second)
	if _, err := adapters.FetchManifest(context.Background(), good); err != nil {
		t.Fatalf("the right token was refused: %v", err)
	}
}

// A malformed request is 400 and is not retried; the model-facing text carries
// no URL.
func TestBadRequestIsClassifiedAndRedacted(t *testing.T) {
	base := startReferenceAdapter(t, "")
	src := adapters.Resolve(adapters.KindWeather,
		adapters.Entry{ID: "reference", Format: adapters.FormatHTTP, BaseURL: base}, nil, adapters.NewHealth())
	set, _ := weather.NewSources([]*adapters.Source{src}, weather.Options{})

	_, err := set.Lookup(context.Background(), "reference", domain.WeatherQuery{Location: "", Days: 1})
	e, ok := adapters.AsError(err)
	if !ok {
		t.Fatalf("got %v, want an adapters.Error", err)
	}
	if e.Kind != adapters.KindRequest || e.Retryable() {
		t.Errorf("kind=%s retryable=%v, want request/false", e.Kind, e.Retryable())
	}
	if strings.Contains(e.Error(), base) {
		t.Errorf("the model-facing error carries the adapter URL: %s", e.Error())
	}
	if !strings.Contains(e.Detail(), base) {
		t.Error("the log-facing detail lost the URL, so the failure is undebuggable")
	}
}
