package main

import (
	"daycore/internal/apipath"

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
// It is also the cross-platform check. The implementation is deliberately NOT
// one code path — Unix replaces its own image with syscall.Exec, Windows spawns
// and exits, because that is what each platform actually has — so this test
// asserts the SHARED outcome (the server comes back as a different instance)
// plus the per-platform one (whether the pid moved), the latter through a
// build-tagged constant that lives next to the implementation making it true.
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
	// ⚠️ PORT is NOT in the environment: godotenv lets the real environment win,
	// so a PORT set here could never be changed by a restart — and "the address
	// moved" is a case this file has to be able to produce. It lives in .env,
	// which the child re-reads.
	writeEnvFile(t, dir, port)
	cmd.Env = append(os.Environ(),
		"HOST=127.0.0.1",
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
	// ⚠️ A FILE, not an io.Writer. Handing exec.Cmd a plain Writer makes it
	// create a pipe and a copying goroutine, and cmd.Wait then blocks until the
	// pipe closes — which a spawned replacement holds open. That would make
	// "did the parent exit" unanswerable, and it is the question this test turns
	// on. A file is inherited as a file descriptor: nothing to copy, and Wait
	// returns the moment the process does.
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	readLogs := func() string {
		b, _ := os.ReadFile(logPath)
		return string(b)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Reaped in the background so the "did it exit" check below sees an exit
	// rather than a zombie — a zombie answers yes to every liveness probe there
	// is, which is what made an earlier version of this assertion vacuous.
	exited := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(exited) }()
	// The child we are about to spawn is NOT this cmd, so cleanup has to kill
	// whatever is listening rather than just this process handle.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		// The replacement is a different process from cmd, so killing the
		// handle above is not enough — find whatever is listening and stop it,
		// or the port stays busy for every later run.
		killListener(t, readLogs())
		assertNothingLeftListening(t, addr)
		if t.Failed() {
			t.Logf("server output:\n%s", readLogs())
		}
	})

	firstID := waitForServer(t, addr, adminToken, 30*time.Second)
	if firstID == "" {
		t.Fatalf("the server never came up. output:\n%s", readLogs())
	}
	firstInherited, _ := inheritedAt(addr, adminToken)

	// ── the button ──────────────────────────────────────────────────────────
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+apipath.Path("/api/admin/restart"), nil)
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
		t.Fatalf("the server did not come back within a minute. output:\n%s", readLogs())
	}
	// The replacement knows it is one, which is what arms its bind retry.
	//
	// ⚠️ Checking for the WORD "restartedFrom" would prove nothing: the first
	// process logs that key too, with an empty value. What has to be there is a
	// NON-EMPTY one, meaning DAYCORE_RESTART_FROM actually crossed into the
	// child — and without it the child would fail its first bind instead of
	// waiting for the parent to let go.
	if out := readLogs(); !restartedFromPID.MatchString(out) {
		t.Errorf("no process logged a non-empty restartedFrom; the restart marker is not reaching the child:\n%s", out)
	}

	// ── the platform's own promise ──────────────────────────────────────────
	//
	// ⚠️ Without this, an implementation that switched Unix to spawn-and-exit
	// would still change the instance id and still pass everything above, while
	// losing the three things exec was chosen for: no coexistence window, no
	// orphan, and pid 1 in a container staying alive.
	var didExit bool
	select {
	case <-exited:
		didExit = true
	case <-time.After(3 * time.Second):
	}
	if parentExitsOnRestart && !didExit {
		t.Errorf("pid %d is still running after the restart; on this platform it should have "+
			"spawned a replacement and exited, leaving it the port", cmd.Process.Pid)
	}
	if !parentExitsOnRestart && didExit {
		t.Errorf("pid %d exited during the restart. On this platform the restart is syscall.Exec, "+
			"which replaces the image IN PLACE — an exit means it is spawning instead, and a "+
			"container running this as pid 1 would have stopped", cmd.Process.Pid)
	}

	// ── the socket was handed over, so nothing was refused ──────────────────
	//
	// ⚠️ Only where exec exists. Windows spawns a replacement that binds for
	// itself, and asserting otherwise there would be asserting a thing the
	// platform cannot do.
	if !parentExitsOnRestart {
		inherited, ok := inheritedAt(addr, adminToken)
		if !ok {
			t.Error("could not ask the replacement whether it inherited the socket")
		} else if !inherited {
			t.Errorf("the replacement bound a fresh socket. The address did not change, so it should "+
				"have adopted the one handed to it — connections arriving during the restart were "+
				"refused instead of queued.\n%s", readLogs())
		}
	}
	// A first boot must NOT claim to have inherited anything: the flag has to
	// mean something, and a constant true would pass every check above.
	if firstInherited {
		t.Error("the FIRST process reported an inherited socket; it had nothing to inherit from")
	}

	t.Logf("restarted: instance %s → %s (pid %d, parent exited=%v)", firstID, secondID, cmd.Process.Pid, didExit)
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

// writeEnvFile puts PORT where a restart can change it.
func writeEnvFile(t *testing.T, dir string, port int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte(fmt.Sprintf("PORT=%d\n", port)), 0o600); err != nil {
		t.Fatal(err)
	}
}

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
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+apipath.Path("/api/admin/health"), nil)
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

// inheritedAt asks whether the process now serving adopted its predecessor's
// listening socket.
func inheritedAt(addr, token string) (bool, bool) {
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+apipath.Path("/api/admin/health"), nil)
	if err != nil {
		return false, false
	}
	req.Header.Set("X-Admin-Token", token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	var body struct {
		Inherited bool `json:"listenerInherited"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, false
	}
	return body.Inherited, true
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
// On Unix there usually is nothing to do: exec keeps the pid, so killing
// cmd.Process already got it. On Windows — and under any mutation that turns
// Unix into spawn-and-exit — the replacement is a DIFFERENT process this test
// has no handle on. Its pid is in the log though: the parent prints it when it
// starts the child, and the child inherits the parent's stdout, which is the
// file this test is reading. Parsing it needs no OS-specific "who owns this
// port" call and stays the same on both platforms.
//
// ⚠️ Leaking the replacement leaks a server process per run and eventually takes
// the port with it. That is not hypothetical: a mutation run left one behind
// for hours, because the mutated code did not print the line this looks for.
// Hence assertNothingLeftListening, which turns a silent leak into a failure.
var restartedFromPID = regexp.MustCompile(`restartedFrom"?[=:]\s*"?\d+`)

var spawnedPID = regexp.MustCompile(`started the replacement process.*?pid=(\d+)`)

func killListener(t *testing.T, logs string) {
	t.Helper()
	for _, m := range spawnedPID.FindAllStringSubmatch(logs, -1) {
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid <= 0 {
			continue
		}
		if p, ferr := os.FindProcess(pid); ferr == nil {
			_ = p.Kill()
		}
	}
}

// Changing the listen address across a restart: the socket is NOT inherited.
//
// This is the condition the handover exists behind, and it is the half that
// fails silently if it is wrong. Inheriting blindly would leave the replacement
// serving the OLD address while its configuration says a new one — a restart
// that looks like it worked and quietly ignored the reason it was asked for.
//
// ⚠️ The decision has to be made by the CHILD, because only the child has read
// the new configuration. A parent that compared would be comparing the config
// against itself, and would always conclude "unchanged".
func TestChangingTheAddressRebindsInsteadOfInheriting(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a real server process")
	}
	bin := buildDaycore(t)
	dir := t.TempDir()
	oldPort, newPort := freePort(t), freePort(t)
	if oldPort == newPort {
		t.Skip("the kernel handed out the same port twice")
	}
	oldAddr := fmt.Sprintf("127.0.0.1:%d", oldPort)
	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	const adminToken = "rebind-e2e-token"

	writeEnvFile(t, dir, oldPort)
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOST=127.0.0.1",
		"DB_TYPE=sqlite",
		"DB_DSN=file:"+filepath.Join(dir, "e2e.db"),
		"ADMIN_TOKEN="+adminToken,
		"APP_ENV=development",
		"JWT_SECRET=e2e-secret-that-is-long-enough-to-pass",
		"COOKIE_SECRET=e2e-cookie-secret-long-enough-too",
		"STATIC_DIR="+filepath.Join(dir, "nothing"),
		"MODELS_CONFIG="+mustAbs(t, "../../config/models.yaml"),
		"OAUTH_CONFIG="+filepath.Join(dir, "no-oauth.yaml"),
		"PROVIDERS_CONFIG="+filepath.Join(dir, "no-providers.yaml"),
	)
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	readLogs := func() string { b, _ := os.ReadFile(logPath); return string(b) }
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		killListener(t, readLogs())
		assertNothingLeftListening(t, oldAddr)
		assertNothingLeftListening(t, newAddr)
		if t.Failed() {
			t.Logf("server output:\n%s", readLogs())
		}
	})

	if waitForServer(t, oldAddr, adminToken, 30*time.Second) == "" {
		t.Fatalf("the server never came up on %s. output:\n%s", oldAddr, readLogs())
	}

	// The operator edits .env by hand — which is the only way a boot-layer knob
	// changes — and presses restart.
	writeEnvFile(t, dir, newPort)

	req, _ := http.NewRequest(http.MethodPost, "http://"+oldAddr+apipath.Path("/api/admin/restart"), nil)
	req.Header.Set("X-Admin-Token", adminToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("restart request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart answered %d", resp.StatusCode)
	}

	// It comes back on the NEW address…
	if waitForServer(t, newAddr, adminToken, 60*time.Second) == "" {
		t.Fatalf("nothing came up on the new address %s. output:\n%s", newAddr, readLogs())
	}
	// …having bound a fresh socket rather than adopting the one it was handed.
	if inherited, ok := inheritedAt(newAddr, adminToken); !ok {
		t.Error("could not ask the replacement whether it inherited the socket")
	} else if inherited {
		t.Errorf("the replacement adopted a socket bound to %s while its configuration says %s — "+
			"it is serving the old address and the restart silently ignored the change", oldAddr, newAddr)
	}
	// And the old address is released, not held by a process that is not
	// serving it. A leaked socket means the next thing to try that port gets
	// EADDRINUSE from a process with no idea it is holding it.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c, derr := net.DialTimeout("tcp", oldAddr, 300*time.Millisecond); derr != nil {
			return // refused: the old socket is gone, which is what should happen
		} else {
			c.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("%s is still accepting connections after the address changed; the inherited socket was "+
		"not closed.\n%s", oldAddr, readLogs())
}

// assertNothingLeftListening fails the test if a server survived its cleanup.
//
// ⚠️ A leak here is invisible without it: the test passes, a daycore process
// keeps running, and the next thing to want that port gets a refusal from
// something nobody remembers starting. It happened — a mutation run left one
// alive for hours.
func assertNothingLeftListening(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		c.Close()
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("something is still listening on %s after cleanup — this test leaked a server process", addr)
}
