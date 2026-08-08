package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func configView(t *testing.T, s *Server) map[string]map[string]any {
	t.Helper()
	rec := adminReq(t, s, http.MethodGet, "/api/admin/config", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settings []map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, s := range body.Settings {
		out[s["key"].(string)] = s
	}
	return out
}

func putConfig(t *testing.T, s *Server, body string) (int, string) {
	t.Helper()
	rec := adminReq(t, s, http.MethodPut, "/api/admin/config", body, func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	return rec.Code, rec.Body.String()
}

// Secrets never leave — not the value, not a prefix, not a length. A masked
// secret still tells an attacker how long it is and whether it changed between
// two reads, and neither is something the console needs to do its job.
func TestAdminConfigNeverReturnsASecret(t *testing.T) {
	s := adminServer(t)
	s.cfg.JWTSecret = "super-secret-signing-key"
	s.cfg.DBDSN = "postgres://user:hunter2@db/daycore"

	rec := adminReq(t, s, http.MethodGet, "/api/admin/config", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	raw := rec.Body.String()
	for _, leak := range []string{"super-secret-signing-key", "hunter2", "the-real-token"} {
		if strings.Contains(raw, leak) {
			t.Errorf("the config screen returned %q", leak)
		}
	}
	view := configView(t, s)
	jwt := view["JWTSecret"]
	if jwt["value"] != nil {
		t.Errorf("JWTSecret carried a value: %v", jwt["value"])
	}
	if jwt["set"] != true {
		t.Error("the console cannot even tell whether the signing key is configured")
	}
	if jwt["editable"] == true {
		t.Error("JWTSecret is editable")
	}
}

// Boot settings are shown and not editable; runtime ones are both.
func TestAdminConfigSeparatesTheTwoLayers(t *testing.T) {
	view := configView(t, adminServer(t))

	boot := view["DBDSN"]
	if boot["layer"] != "boot" || boot["editable"] == true {
		t.Errorf("DBDSN: layer=%v editable=%v", boot["layer"], boot["editable"])
	}
	run := view["MaxUploadBytes"]
	if run["layer"] != "runtime" || run["editable"] != true {
		t.Errorf("MaxUploadBytes: layer=%v editable=%v", run["layer"], run["editable"])
	}
	if run["source"] != "env" {
		t.Errorf("an unoverridden knob reports source=%v, want env", run["source"])
	}
	// A knob that is runtime by nature but baked in at construction must say so,
	// or the console implies a change is live when it is waiting for a restart.
	if view["RateLimitPerMin"]["requiresRestart"] != true {
		t.Error("RateLimitPerMin does not report requiresRestart, so the console would claim the change took effect")
	}
	if view["MaxUploadBytes"]["requiresRestart"] == true {
		t.Error("a genuinely hot knob was marked requiresRestart")
	}
	// A surprising classification carries its reason to the operator, who should
	// not have to read Go to find out why a field is greyed out.
	if why, _ := view["AllowedOrigins"]["why"].(string); why == "" {
		t.Error("AllowedOrigins is boot-layer for a non-obvious reason and the console shows no explanation")
	}
}

// The override wins over the environment seed, and the whole point is that it
// takes effect without a restart.
func TestOverrideTakesEffectAndResets(t *testing.T) {
	s := adminServer(t)
	s.cfg.MaxUploadBytes = 32 << 20
	if err := s.ReloadSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.runtime().MaxUploadBytes; got != 32<<20 {
		t.Fatalf("seed = %d", got)
	}

	if code, body := putConfig(t, s, `{"settings":{"MaxUploadBytes":"1048576"}}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if got := s.runtime().MaxUploadBytes; got != 1<<20 {
		t.Errorf("after the override the process still reads %d — the console saved something nothing uses", got)
	}
	// The boot config is untouched: it is what "reset" restores to, and it is
	// shared by everything constructed from it.
	if s.cfg.MaxUploadBytes != 32<<20 {
		t.Error("applying an override mutated the boot config")
	}
	if configView(t, s)["MaxUploadBytes"]["source"] != "override" {
		t.Error("the console cannot tell that this value now comes from an override")
	}

	// null resets to the seed — and is a different request from "".
	if code, body := putConfig(t, s, `{"settings":{"MaxUploadBytes":null}}`); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, body)
	}
	if got := s.runtime().MaxUploadBytes; got != 32<<20 {
		t.Errorf("reset left %d, want the environment seed", got)
	}
}

// A row the console displays and the process ignores is the worst outcome
// available here, so a boot or secret key is refused rather than stored.
func TestAdminConfigRefusesWhatItCannotHonour(t *testing.T) {
	s := adminServer(t)
	cases := []struct{ name, body, wantErr string }{
		{"a boot setting", `{"settings":{"DBDSN":"postgres://x"}}`, "not_editable"},
		{"a secret", `{"settings":{"JWTSecret":"x"}}`, "not_editable"},
		{"a typo", `{"settings":{"MaxUploadByte":"1"}}`, "unknown_setting"},
		{"a value of the wrong type", `{"settings":{"MaxUploadBytes":"lots"}}`, "bad_value"},
		{"an unparseable duration", `{"settings":{"AIRequestTimeout":"soon"}}`, "bad_value"},
		{"nothing at all", `{"settings":{}}`, "bad_request"},
	}
	for _, tc := range cases {
		code, body := putConfig(t, s, tc.body)
		if code != http.StatusBadRequest || !strings.Contains(body, tc.wantErr) {
			t.Errorf("%s: %d %s, want 400 %s", tc.name, code, body, tc.wantErr)
		}
	}
	// And nothing was stored by any of them.
	rows, err := s.store.Settings().All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("a refused request stored %d rows: %+v", len(rows), rows)
	}
}

// The whole request is validated before any of it is written: a partial apply
// leaves the operator with some edits in place and no way to tell which.
func TestAdminConfigIsAllOrNothing(t *testing.T) {
	s := adminServer(t)
	code, _ := putConfig(t, s, `{"settings":{"MaxUploadBytes":"1048576","DBDSN":"postgres://x"}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("a request with one illegal key returned %d", code)
	}
	rows, _ := s.store.Settings().All(context.Background())
	if len(rows) != 0 {
		t.Errorf("the legal half of a refused request was written anyway: %+v", rows)
	}
}

// requiresRestart is reported per key, because the operator's next question is
// which of their changes is waiting.
func TestAdminConfigReportsWhichChangesWait(t *testing.T) {
	s := adminServer(t)
	code, body := putConfig(t, s, `{"settings":{"MaxUploadBytes":"1048576","RateLimitPerMin":"10"}}`)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	var res struct {
		RequiresRestart []string `json:"requiresRestart"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.RequiresRestart) != 1 || res.RequiresRestart[0] != "RateLimitPerMin" {
		t.Errorf("requiresRestart = %v, want exactly [RateLimitPerMin]", res.RequiresRestart)
	}
}

// The config screen must survive with no database — it is the one thing a
// degraded process is for, and it is where the operator reads what is wrong.
func TestAdminConfigReadsInDegradedMode(t *testing.T) {
	s := degradedServer(t)
	rec := adminReq(t, s, http.MethodGet, "/api/admin/config", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("the config screen is unavailable in degraded mode: %d %s", rec.Code, rec.Body.String())
	}
	// Writing is not, and says so rather than failing obscurely.
	w := adminReq(t, s, http.MethodPut, "/api/admin/config", `{"settings":{"MaxUploadBytes":"1"}}`, func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT in degraded mode: %d, want 503", w.Code)
	}
}

// A stored row that no longer applies — left by an older build, or a
// hand-written INSERT — must not cost the operator every other override.
func TestOneBadRowDoesNotPoisonTheRest(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	if err := s.store.Settings().Set(ctx, "MaxUploadBytes", "4096"); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Settings().Set(ctx, "JWTSecret", "smuggled"); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Settings().Set(ctx, "AgentMaxRounds", "not a number"); err != nil {
		t.Fatal(err)
	}

	if err := s.ReloadSettings(ctx); err != nil {
		t.Fatalf("one bad row failed the whole reload: %v", err)
	}
	if got := s.runtime().MaxUploadBytes; got != 4096 {
		t.Errorf("the good override was lost: %d", got)
	}
	if s.runtime().JWTSecret == "smuggled" {
		t.Error("a secret written straight into the table was applied — Apply must re-check, not trust the row")
	}
	if s.runtime().AgentMaxRounds != s.cfg.AgentMaxRounds {
		t.Error("an unparseable value changed the field")
	}
}
