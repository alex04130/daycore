package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daycore/internal/ai"
	"daycore/internal/auth"

	_ "daycore/internal/ai/formats/openai"
)

// modelServer is an admin server whose catalog has two entries: one with a key
// from the environment and one with none.
func modelServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("TEST_MODEL_KEY", "sk-sentinel-abc123")
	if os.Getenv("TEST_MODEL_KEY") == "" {
		t.Fatal("the sentinel is empty; the leak assertions below would assert nothing")
	}
	path := filepath.Join(t.TempDir(), "models.yaml")
	if err := os.WriteFile(path, []byte(`models:
  - id: chat
    format: openai
    base_url: https://api.example.invalid/v1
    model: some-upstream-name
    api_key_env: TEST_MODEL_KEY
    tools: true
  - id: keyless
    format: openai
    base_url: https://other.example.invalid/v1
    model: another
    vision: true
`), 0644); err != nil {
		t.Fatal(err)
	}
	cat, err := ai.LoadCatalog(path, "chat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	s := adminServer(t)
	s.catalog = cat
	s.cfg.ModelsConfigPath = path
	return s
}

func adminGet(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	rec := adminReq(t, s, http.MethodGet, path, "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	return rec.Code, rec.Body.String()
}

// The screen answers the two questions setup actually fails on: is the key set,
// and which upstream is this hitting. Neither is answerable from a browser
// today.
func TestModelsScreenShowsWhatSetupFailsOn(t *testing.T) {
	s := modelServer(t)
	code, body := adminGet(t, s, "/api/admin/models")
	if code != http.StatusOK {
		t.Fatalf("GET models: %d %s", code, body)
	}
	if strings.Contains(body, "sk-sentinel-abc123") {
		t.Error("the API key value reached the models screen")
	}

	var res struct {
		Models     []ai.ModelDetail `json:"models"`
		ConfigPath string           `json:"configPath"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if res.ConfigPath == "" {
		t.Error("nothing on this screen is writable and it does not say which file to edit")
	}
	byID := map[string]ai.ModelDetail{}
	for _, m := range res.Models {
		byID[m.ID] = m
	}

	chat := byID["chat"]
	if !chat.KeySet {
		t.Error("a model with a key in the environment reports keySet=false")
	}
	if chat.APIKeyEnv != "TEST_MODEL_KEY" {
		t.Errorf("apiKeyEnv = %q — the variable NAME is not a credential and is what the operator needs", chat.APIKeyEnv)
	}
	if chat.Model != "some-upstream-name" {
		t.Errorf("model = %q; id and the vendor's name are two fields on purpose", chat.Model)
	}
	if chat.BaseURL == "" {
		t.Error("no base URL — 'which gateway is this hitting' is unanswerable")
	}
	if byID["keyless"].KeySet {
		t.Error("a model with no key configured reports keySet=true")
	}
}

// Roles are derived, so they cannot disagree with what the process is doing.
// The planner in particular falls back to the chat model, and reporting only
// where it was NAMED would leave that slot looking empty.
func TestRolesAreDerivedIncludingThePlannerFallback(t *testing.T) {
	s := modelServer(t)
	_, body := adminGet(t, s, "/api/admin/models")
	var res struct {
		Models []ai.ModelDetail `json:"models"`
	}
	json.Unmarshal([]byte(body), &res)

	roles := map[string][]string{}
	for _, m := range res.Models {
		roles[m.ID] = m.Roles
	}
	got := strings.Join(roles["chat"], ",")
	if !strings.Contains(got, "chat") {
		t.Errorf("the default chat model does not report the chat role: %v", roles["chat"])
	}
	if !strings.Contains(got, "planner") {
		t.Errorf("no model reports the planner role, but the planner falls back to chat: %v", roles["chat"])
	}
	// The vision-capable one is auto-picked when DEFAULT_VISION_MODEL is unset.
	if !strings.Contains(strings.Join(roles["keyless"], ","), "vision") {
		t.Errorf("the auto-picked vision model does not report the role: %v", roles["keyless"])
	}
}

// A failed call is 200 with ok:false, not an error status: the request to THIS
// server succeeded and found something out. A 502 would make the console show
// "the admin API is broken" over what is, in fact, the answer.
func TestModelTestReportsFailureAsAnAnswer(t *testing.T) {
	s := modelServer(t)
	rec := adminReq(t, s, http.MethodPost, "/api/admin/models/chat/test", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
		r.SetPathValue("id", "chat")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("a model that cannot be reached returned %d; the check itself worked", rec.Code)
	}
	var res struct {
		OK        bool   `json:"ok"`
		Model     string `json:"model"`
		ElapsedMs int64  `json:"elapsedMs"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	// api.example.invalid does not resolve, so this must report a failure.
	if res.OK {
		t.Error("an unreachable host reported ok")
	}
	if res.Model != "chat" || res.Message == "" {
		t.Errorf("incomplete result: %+v", res)
	}
	if strings.Contains(rec.Body.String(), "sk-sentinel-abc123") {
		t.Error("the API key reached the test result")
	}
}

func TestModelTestRefusesAnUnknownModel(t *testing.T) {
	s := modelServer(t)
	rec := adminReq(t, s, http.MethodPost, "/api/admin/models/nope/test", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
		r.SetPathValue("id", "nope")
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown model returned %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nope") {
		t.Errorf("the error does not name what was asked for: %s", rec.Body.String())
	}
}

// The redirect URI is the point of the OAuth screen: a mismatch there is the
// most common setup failure, the vendor's error says nothing useful, and the
// value is derived from a config nobody can see from a browser.
func TestOAuthScreenGivesTheCallbackURLAndNoSecret(t *testing.T) {
	s := adminServer(t)
	path := filepath.Join(t.TempDir(), "oauth.yaml")
	if err := os.WriteFile(path, []byte(`providers:
  - name: google
    client_id: "google-client-id"
    client_secret: "shhh-sentinel"
  - name: unconfigured
    client_id: ""
    client_secret: ""
`), 0644); err != nil {
		t.Fatal(err)
	}
	mgr, err := auth.LoadOAuthProviders(path, "https://day.example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.oauth = mgr
	s.cfg.OAuthConfigPath = path

	code, body := adminGet(t, s, "/api/admin/oauth")
	if code != http.StatusOK {
		t.Fatalf("GET oauth: %d %s", code, body)
	}
	if strings.Contains(body, "shhh-sentinel") {
		t.Error("the client secret reached the OAuth screen")
	}
	var res struct {
		Providers []auth.ProviderView `json:"providers"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Providers) != 1 {
		t.Fatalf("got %d providers; one has an empty client_id and is skipped at load", len(res.Providers))
	}
	g := res.Providers[0]
	if g.RedirectURI != "https://day.example.com/api/auth/oauth/google/callback" {
		t.Errorf("redirectUri = %q — this is the field the whole screen exists for", g.RedirectURI)
	}
	if !g.SecretSet {
		t.Error("a provider with a secret reports secretSet=false")
	}
	if g.ClientID != "google-client-id" {
		t.Errorf("clientId = %q; it is not a secret and the operator needs it", g.ClientID)
	}
	if !g.Preset {
		t.Error("google is a preset and does not say so, which is why a two-line entry works")
	}
}

// Both screens are admin-only.
func TestModelAndOAuthScreensNeedACredential(t *testing.T) {
	s := modelServer(t)
	for _, path := range []string{"/api/admin/models", "/api/admin/oauth"} {
		rec := adminReq(t, s, http.MethodGet, path, "", func(r *http.Request) {})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a credential: %d, want 401", path, rec.Code)
		}
	}
	rec := adminReq(t, s, http.MethodPost, "/api/admin/models/chat/test", "", func(r *http.Request) {
		r.SetPathValue("id", "chat")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the test endpoint without a credential: %d, want 401", rec.Code)
	}
}
