package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"daycore/internal/adapters"
	"daycore/internal/weather"

	// Registers the built-in providers the entries below name. Without these the
	// factories map is empty and every source is dropped with a reason — which is
	// itself the right behaviour, just not what these tests are about.
	_ "daycore/internal/weather/openmeteo"
	_ "daycore/internal/weather/wttrin"
)

// providerServer is an admin server with two weather sources, one of which
// carries an operator description in the file.
func providerServer(t *testing.T) *Server {
	t.Helper()
	s := adminServer(t)
	entries := []adapters.Entry{
		{ID: "open-meteo", Format: adapters.FormatBuiltin, Impl: "open-meteo"},
		{ID: "wttr", Format: adapters.FormatBuiltin, Impl: "wttr",
			Description: map[string]string{"zh-CN": "文件里写的", "en-US": "from the file"}},
	}
	resolved := make([]*adapters.Source, 0, len(entries))
	for _, e := range entries {
		resolved = append(resolved, adapters.Resolve(adapters.KindWeather, e, nil, adapters.NewHealth()))
	}
	ws, problems := weather.NewSources(resolved, weather.Options{})
	if len(problems) > 0 {
		t.Fatalf("building sources: %v", problems)
	}
	s.weather = ws
	return s
}

func getProviders(t *testing.T, s *Server) map[string]map[string]any {
	t.Helper()
	rec := adminReq(t, s, http.MethodGet, "/api/admin/providers", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET providers: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, p := range body.Providers {
		out[p["kind"].(string)+"/"+p["id"].(string)] = p
	}
	return out
}

func putProviders(t *testing.T, s *Server, body string) (int, string) {
	t.Helper()
	rec := adminReq(t, s, http.MethodPut, "/api/admin/providers", body, func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	return rec.Code, rec.Body.String()
}

// The screen shows what it cannot change, and says which file to edit for it.
// Without that, the greyed-out rows are a dead end.
func TestProvidersScreenShowsTheUneditableAndWhereItLives(t *testing.T) {
	s := providerServer(t)
	rec := adminReq(t, s, http.MethodGet, "/api/admin/providers", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	var body struct {
		ConfigPath string `json:"configPath"`
		Instance   string `json:"instance"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.ConfigPath == "" {
		t.Error("the response does not say which file holds the fields this screen cannot touch")
	}
	if body.Instance == "" {
		t.Error("no instance id — health is per process, so two consoles disagree with no way to tell which machine")
	}

	view := getProviders(t, s)["weather/open-meteo"]
	if view["format"] != "builtin" || view["enabled"] != true {
		t.Errorf("unexpected view: %+v", view)
	}
	editable, _ := view["editable"].([]any)
	got := make([]string, 0, len(editable))
	for _, e := range editable {
		got = append(got, e.(string))
	}
	if strings.Join(got, ",") != "enabled,baseUrl,description,approved" {
		t.Errorf("editable = %v", got)
	}
	// ⚠️ This assertion used to read "base_url must never be in it — it is the
	// SSRF entrance". That rule was reversed on 2026-08-09, and the reasoning is
	// worth keeping because the old sentence was persuasive and wrong:
	// "only shell access may set it" was never the defence. The same batch had
	// already concluded, about this same field, that anybody who can edit
	// providers.yaml already has the machine.
	//
	// What defends is independent of who typed the value — link-local refused by
	// ValidateBaseURL, redirects refused by the client — and both run on a
	// console value exactly as they run on a file value. Those are asserted in
	// TestBaseURLIsEditableButStillValidated, which is where this test's teeth
	// moved to.
	for _, mustNot := range []string{"format", "impl", "tokenEnv", "id"} {
		if strings.Contains(strings.Join(got, ","), mustNot) {
			t.Errorf("%s became editable; the process built something out of it at startup", mustNot)
		}
	}
}

// No secret, and nothing standing in for one.
func TestProvidersScreenNeverCarriesAToken(t *testing.T) {
	t.Setenv("MY_ADAPTER_TOKEN", "sentinel-abc123")
	if len("sentinel-abc123") == 0 {
		t.Fatal("the sentinel is empty; this test would assert nothing")
	}
	s := adminServer(t)
	src := adapters.Resolve(adapters.KindWeather, adapters.Entry{
		ID: "wx", Format: adapters.FormatHTTP, BaseURL: "https://wx.example.com", TokenEnv: "MY_ADAPTER_TOKEN",
	}, nil, adapters.NewHealth())
	ws, _ := weather.NewSources([]*adapters.Source{src}, weather.Options{})
	s.weather = ws

	rec := adminReq(t, s, http.MethodGet, "/api/admin/providers", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if strings.Contains(rec.Body.String(), "sentinel-abc123") {
		t.Error("the token value reached the providers screen")
	}
	v := getProviders(t, s)["weather/wx"]
	if v["tokenSet"] != true {
		t.Error("the console cannot tell whether the token is configured")
	}
	if v["tokenEnv"] != "MY_ADAPTER_TOKEN" {
		t.Errorf("tokenEnv = %v — the variable NAME is not a credential and the operator needs it", v["tokenEnv"])
	}
}

// Writing a row the process does not pick up is the failure this whole layering
// exists to prevent: the console reports a change and the process keeps using
// the old value.
func TestDisablingASourceTakesEffectImmediately(t *testing.T) {
	s := providerServer(t)
	if got := strings.Join(s.weather.IDs(), ","); got != "open-meteo,wttr" {
		t.Fatalf("setup: %s", got)
	}

	code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr","enabled":false}]}`)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if got := strings.Join(s.weather.IDs(), ","); got != "open-meteo" {
		t.Errorf("the tool enum still has wttr in it: %s", got)
	}
	// And it survives a reload from the table, i.e. the row was actually stored.
	if err := s.ReloadProviders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.weather.IDs(), ","); got != "open-meteo" {
		t.Errorf("after reload: %s — the change did not reach the table", got)
	}
	// A disabled source is still SHOWN. It is the one the operator is about to
	// turn back on.
	if _, ok := getProviders(t, s)["weather/wttr"]; !ok {
		t.Error("a disabled source vanished from the console")
	}
}

// Approval is granted against specific words, so editing them revokes it. This
// is the assertion that keeps the gate from being decorative.
func TestEditingADescriptionThroughTheConsoleRevokesApproval(t *testing.T) {
	s := providerServer(t)
	src := s.weather.Source("wttr")

	code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr",
		"description":{"zh-CN":"运维批准的","en-US":"approved text"},"approved":true}]}`)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if got := src.PromptDescription("en-US"); got != "approved text" {
		t.Fatalf("approved text did not reach the prompt: %q", got)
	}

	// Change the words without re-approving.
	code, body = putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr",
		"description":{"zh-CN":"改过了","en-US":"edited"}}]}`)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if getProviders(t, s)["weather/wttr"]["approved"] == true {
		t.Error("editing the description kept its approval")
	}
	if got := src.PromptDescription("en-US"); strings.Contains(got, "edited") {
		t.Errorf("the edited text reached the prompt anyway: %q", got)
	}
}

// The endpoint refuses what it cannot honour, and nothing is stored by a
// refused request.
func TestProvidersPutRefusesWhatItCannotHonour(t *testing.T) {
	s := providerServer(t)
	cases := []struct{ name, body, want string }{
		{"an unknown source", `{"providers":[{"kind":"weather","id":"nope","enabled":false}]}`, "nope"},
		{"an unknown kind", `{"providers":[{"kind":"storage","id":"x","enabled":false}]}`, "storage"},
		{"one locale only", `{"providers":[{"kind":"weather","id":"wttr","description":{"zh-CN":"只有中文"}}]}`, "zh-CN"},
		{"approving nothing", `{"providers":[{"kind":"weather","id":"open-meteo","approved":true}]}`, "open-meteo"},
		{"nothing at all", `{"providers":[]}`, ""},
	}
	for _, tc := range cases {
		code, body := putProviders(t, s, tc.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", tc.name, code, body)
			continue
		}
		if tc.want != "" && !strings.Contains(body, tc.want) {
			t.Errorf("%s: the error does not name what was wrong: %s", tc.name, body)
		}
	}
	rows, err := s.store.ProviderOverrides().All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("a refused request stored %d rows: %+v", len(rows), rows)
	}
}

// A patch is sparse. A console editing one checkbox must not have to remember
// to resend the description, because forgetting would be silent.
func TestAPatchLeavesUnsentFieldsAlone(t *testing.T) {
	s := providerServer(t)
	if code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr",
		"description":{"zh-CN":"甲","en-US":"a"},"approved":true}]}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	// Now toggle only `enabled`.
	if code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr","enabled":false}]}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	v := getProviders(t, s)["weather/wttr"]
	if v["approved"] != true {
		t.Error("toggling enabled dropped the approval")
	}
	desc, _ := v["description"].(map[string]any)
	if desc["en-US"] != "a" {
		t.Errorf("toggling enabled dropped the description: %v", desc)
	}
}

// The whole request is validated before any of it is written — there is no
// transaction, and a partial apply leaves the operator with some edits in place
// and no way to tell which.
func TestProvidersPutIsAllOrNothing(t *testing.T) {
	s := providerServer(t)
	code, _ := putProviders(t, s, `{"providers":[
		{"kind":"weather","id":"wttr","enabled":false},
		{"kind":"weather","id":"nope","enabled":false}]}`)
	if code != http.StatusBadRequest {
		t.Fatalf("a request naming one unknown source returned %d", code)
	}
	rows, _ := s.store.ProviderOverrides().All(context.Background())
	if len(rows) != 0 {
		t.Errorf("the legal half of a refused request was written: %+v", rows)
	}
	if got := strings.Join(s.weather.IDs(), ","); got != "open-meteo,wttr" {
		t.Errorf("the live sources changed anyway: %s", got)
	}
}

// The console must survive with no database for reads — it is where the
// operator looks to find out what is wrong — but it cannot write the override
// table, and it says so rather than failing obscurely.
func TestProvidersReadInDegradedModeButCannotWrite(t *testing.T) {
	s := degradedServer(t)
	ws, _ := weather.NewSources([]*adapters.Source{
		adapters.Resolve(adapters.KindWeather, adapters.Entry{ID: "open-meteo", Format: adapters.FormatBuiltin, Impl: "open-meteo"}, nil, adapters.NewHealth()),
	}, weather.Options{})
	s.weather = ws

	rec := adminReq(t, s, http.MethodGet, "/api/admin/providers", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("the providers screen is unavailable in degraded mode: %d", rec.Code)
	}
	code, _ := putProviders(t, s, `{"providers":[{"kind":"weather","id":"open-meteo","enabled":false}]}`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("PUT in degraded mode: %d, want 503", code)
	}
}

// base_url is editable as of 2026-08-09, and the validation that actually holds
// the SSRF line runs on the console's value exactly as it ran on the file's.
//
// The old rule was "only shell access may change this". That reasoning did not
// hold up: the same batch concluded, about the same field, that an attacker who
// can edit providers.yaml already has the machine. What defends is independent
// of who typed it — link-local refused here, redirects refused in the client.
func TestBaseURLIsEditableButStillValidated(t *testing.T) {
	s := adminServer(t)
	src := adapters.Resolve(adapters.KindWeather, adapters.Entry{
		ID: "wx", Format: adapters.FormatHTTP, BaseURL: "https://old.example.com",
	}, nil, adapters.NewHealth())
	ws, problems := weather.NewSources([]*adapters.Source{src}, weather.Options{})
	if len(problems) > 0 {
		t.Fatalf("setup: %v", problems)
	}
	s.weather = ws

	// A legitimate move.
	if code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wx","baseUrl":"https://new.example.com"}]}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if got := s.weather.Source("wx").BaseURL(); got != "https://new.example.com" {
		t.Errorf("the address is still %q — the console reported a change the process ignored", got)
	}
	// The file's value stays readable, because "revert to the file" needs to
	// know what that means.
	if got := getProviders(t, s)["weather/wx"]["fileBaseUrl"]; got != "https://old.example.com" {
		t.Errorf("fileBaseUrl = %v; the console cannot offer a revert without it", got)
	}

	// The checks that matter still refuse, and they refuse a console value the
	// same way they refuse a file value.
	for _, tc := range []struct{ name, url, want string }{
		{"link-local", "http://169.254.169.254/latest/meta-data/", "link-local"},
		{"embedded credentials", "https://u:p@wx.example.com", "credentials"},
		{"a non-http scheme", "file:///etc/passwd", "scheme"},
	} {
		code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wx","baseUrl":"`+tc.url+`"}]}`)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", tc.name, code, body)
			continue
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s: the error does not say why: %s", tc.name, body)
		}
	}
	// And none of the refusals moved it.
	if got := s.weather.Source("wx").BaseURL(); got != "https://new.example.com" {
		t.Errorf("a refused address changed the source: %q", got)
	}

	// "" returns the source to the file.
	if code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wx","baseUrl":""}]}`); code != http.StatusOK {
		t.Fatalf("revert: %d %s", code, body)
	}
	if got := s.weather.Source("wx").BaseURL(); got != "https://old.example.com" {
		t.Errorf("after clearing the override the address is %q, want the file's", got)
	}
}

// Moving a source resets what this process learned about it.
//
// The health belongs to the machine at the OLD address. Carrying it over would
// mark a freshly pointed adapter down before the first call, on evidence about
// a server it has never spoken to — or, worse, vouch for an address nobody has
// tried.
func TestMovingASourceResetsItsHealth(t *testing.T) {
	s := adminServer(t)
	src := adapters.Resolve(adapters.KindWeather, adapters.Entry{
		ID: "wx", Format: adapters.FormatHTTP, BaseURL: "https://old.example.com",
	}, nil, adapters.NewHealth())
	ws, _ := weather.NewSources([]*adapters.Source{src}, weather.Options{})
	s.weather = ws

	for i := 0; i < adapters.FlipAfter; i++ {
		s.weather.Source("wx").Health.Observe(errors.New("old server is down"), time.Now())
	}
	if s.weather.Source("wx").Health.Up() {
		t.Fatal("setup: the source should be down")
	}

	if code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wx","baseUrl":"https://new.example.com"}]}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if !s.weather.Source("wx").Health.Up() {
		t.Error("the new address inherited the old one's failures — it has never been called")
	}

	// Toggling something else must NOT reset it: that is the case
	// ApplyOverride is deliberately in-place for.
	for i := 0; i < adapters.FlipAfter; i++ {
		s.weather.Source("wx").Health.Observe(errors.New("down"), time.Now())
	}
	if code, _ := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wx","enabled":true}]}`); code != http.StatusOK {
		t.Fatal("enable toggle failed")
	}
	if s.weather.Source("wx").Health.Up() {
		t.Error("toggling enabled reset the health, so a known-dead source silently rejoined the tool band")
	}
}

// A built-in source has no address to change, and saying so is better than
// storing a value nothing reads.
func TestABuiltinSourceRefusesAnAddress(t *testing.T) {
	s := providerServer(t)
	code, body := putProviders(t, s, `{"providers":[{"kind":"weather","id":"wttr","baseUrl":"https://example.com"}]}`)
	if code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", code)
	}
	if !strings.Contains(body, "wttr") {
		t.Errorf("the error does not name the source: %s", body)
	}
}
