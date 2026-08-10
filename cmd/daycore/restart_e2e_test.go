package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The restart button, against a real process.
//
// # Why this test exists at all
//
// Everything else about the restart is a handler test with a fake restarter,
// and those prove the HTTP half. They cannot prove the half that matters: that
// a process which shuts itself down comes BACK. That claim is only true if a
// real binary really does it, and it is the claim whose failure means somebody
// has to walk to the machine.
//
// It is also the Windows/Linux consistency check the design was chosen for. The
// implementation is one code path with no build tags and no SysProcAttr, so
// this test compiles and runs the same on both — if it ever needs a
// `runtime.GOOS` branch, that is the signal the single-path property was lost.
//
// # What it does
//
//	build the binary once → start it on a free port → wait for it to serve
//	→ POST /api/admin/restart → watch for the address answering again
//	→ assert the INSTANCE ID changed
//
// ⚠️ The instance assertion is the whole test. Without it, an implementation
// that answers 200 and does nothing is indistinguishable from one that works.
//
// The id comes from GET /api/admin/health, which already reports it: it is a
// fresh uuid per process, generated in-process and never from configuration
// (leader.go says why). Better than a pid, and it needs no new field on a
// public endpoint — adding one to /api/healthz would have widened a disclosure
// surface docs/AUTH.md deliberately enumerates.
func TestTheProcessReallyRestartsItself(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a real server process")
	}
	bin := buildDaycore(t)
	dir := t.TempDir()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	const adminToken = "restart-e2e-token"

	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOST=127.0.0.1",
		fmt.Sprintf("PORT=%d", port),
		"DB_TYPE=sqlite",
		"DB_DSN=file:"+filepath.Join(dir, "e2e.db"),
		"ADMIN_TOKEN="+adminToken,
		"APP_ENV=development",
		"JWT_SECRET=e2e-secret-that-is-long-enough-to-pass",
		"COOKIE_SECRET=e2e-cookie-secret-long-enough-too",
		"STATIC_DIR="+filepath.Join(dir, "nothing"),
		// The repository's own config, by absolute path. The child is started
		// with cmd.Dir = dir, so a relative default would resolve against the
		// temp directory — and pointing MODELS_CONFIG somewhere real is the
		// difference between testing the restart and testing the boot sequence.
		"MODELS_CONFIG="+mustAbs(t, "../../config/models.yaml"),
		"OAUTH_CONFIG="+filepath.Join(dir, "no-oauth.yaml"),
		"PROVIDERS_CONFIG="+filepath.Join(dir, "no-providers.yaml"),
	)
	var logs safeBuffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The child we are about to spawn is NOT this cmd, so cleanup has to kill
	// whatever is listening rather than just this process handle.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		// The replacement is a different process from cmd, so killing the
		// handle above is not enough — find whatever is listening and stop it,
		// or the port stays busy for every later run.
		killListener(t, &logs)
		if t.Failed() {
			t.Logf("server output:\n%s", logs.String())
		}
	})

	firstID := waitForServer(t, addr, adminToken, 30*time.Second)
	if firstID == "" {
		t.Fatalf("the server never came up. output:\n%s", logs.String())
	}

	// ── the button ──────────────────────────────────────────────────────────
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/admin/restart", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Admin-Token", adminToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("restart request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// ⚠️ The response has to arrive BEFORE the shutdown. If this ever starts
	// failing with a connection error rather than a status code, the ordering in
	// main.go was inverted and the operator now sees a network error where an
	// answer belongs.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart answered %d: %s", resp.StatusCode, body)
	}

	// ── it comes back, and it is not the same process ───────────────────────
	secondID := waitForNewInstance(t, addr, adminToken, firstID, 60*time.Second)
	if secondID == "" {
		t.Fatalf("the server did not come back within a minute. output:\n%s", logs.String())
	}
	// The replacement knows it is one, which is what arms its bind retry.
	//
	// ⚠️ Checking for the WORD "restartedFrom" would prove nothing: the first
	// process logs that key too, with an empty value. What has to be there is a
	// NON-EMPTY one, meaning DAYCORE_RESTART_FROM actually crossed into the
	// child — and without it the child would fail its first bind instead of
	// waiting for the parent to let go.
	if out := logs.String(); !restartedFromPID.MatchString(out) {
		t.Errorf("no process logged a non-empty restartedFrom; the restart marker is not reaching the child:\n%s", out)
	}
	t.Logf("restarted: instance %s → %s", firstID, secondID)
}

// buildDaycore compiles the binary once for this test.
//
// Compiled and executed directly rather than run with `go run`: `go run` leaves
// its own process holding the child's stderr, so a test that waits for output
// waits for the wrapper too. That cost this repository sixty seconds a run once
// already (internal/adapters/e2e_test.go).
var buildOnce = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "daycore-restart-e2e")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "daycore")
	if _, err := os.Stat("/proc/version"); err != nil {
		bin += ".exe" // best-effort; exec.Command does not care, Windows does
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %v\n%s", err, out)
	}
	return bin, nil
})

func mustAbs(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func buildDaycore(t *testing.T) string {
	t.Helper()
	bin, err := buildOnce()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// freePort asks the kernel for one and lets it go.
//
// ⚠️ Inherently racy — something else can take it in between — and the honest
// alternative (start on :0 and read back the port) does not exist here, because
// the whole point is that the SAME address is reclaimed by a different process.
// A fixed port would be worse: two runs of this test would collide.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// instanceAt asks the running server which process it is.
//
// GET /api/admin/health, not /api/healthz: the instance id is admin-only, which
// is right — it is an operational detail, and this test has the token anyway.
func instanceAt(addr, token string) string {
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/api/admin/health", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-Admin-Token", token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct {
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Instance
}

func waitForServer(t *testing.T, addr, token string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if id := instanceAt(addr, token); id != "" {
			return id
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

// waitForNewInstance waits for the address to answer as a DIFFERENT process.
//
// It does not first wait for the port to go quiet: on a fast machine the
// replacement can be listening before this loop runs, and insisting on
// observing the gap would be testing this test's own timing.
func waitForNewInstance(t *testing.T, addr, token, old string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if id := instanceAt(addr, token); id != "" && id != old {
			return id
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ""
}

// killListener stops the replacement process this test caused to exist.
//
// The replacement was spawned by the server under test, not by this test, so
// there is no process handle to wait on. Its pid IS in the log though: the
// parent prints it when it starts the child, and the child inherits the
// parent's stdout — which is the same buffer this test is reading. Parsing it
// is exact, needs no OS-specific "who owns this port" call, and therefore stays
// the same on both platforms this feature has to work on.
//
// ⚠️ Leaking the replacement would leak a server process per run and eventually
// take the port with it.
var restartedFromPID = regexp.MustCompile(`restartedFrom"?[=:]\s*"?\d+`)

var spawnedPID = regexp.MustCompile(`started the replacement process.*?pid=(\d+)`)

func killListener(t *testing.T, logs *safeBuffer) {
	t.Helper()
	for _, m := range spawnedPID.FindAllStringSubmatch(logs.String(), -1) {
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid <= 0 {
			continue
		}
		if p, ferr := os.FindProcess(pid); ferr == nil {
			_ = p.Kill()
		}
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
