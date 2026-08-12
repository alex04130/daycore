package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"daycore/internal/auth"
	"daycore/internal/domain"
	"daycore/internal/theme"
)

// The third tier end to end: a frontend asks, a person agrees, and only then
// does anything validate against it.
//
// ⚠️ The assertion that carries the design is the middle one — between the
// proposal and the approval, the kind must be REFUSED. A tier that installed on
// arrival would be a tier where any frontend can define what "valid" means on
// this deployment.
func TestAProposedKindValidatesNothingUntilSomebodyAgrees(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()

	_, out := handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{
		"kinds":[{"name":"spring","pattern":"[0-9.]+ [0-9.]+","description":"刚度 和 阻尼"}],
		"tokens":[{"name":"--motion","kind":"spring"},{"name":"--primary","kind":"color"}]}}`)

	// It is stored, unapproved, and attributed.
	k, err := s.store.ThemeKinds().Get(ctx, "spring")
	if err != nil {
		t.Fatalf("the proposal was not recorded: %v", err)
	}
	if k.Approved {
		t.Fatal("a proposed kind arrived APPROVED — any frontend could define what valid means here")
	}
	if k.ProposedBy != "liuli" {
		t.Errorf("proposedBy is %q; six months later this is the only answer to why this kind exists", k.ProposedBy)
	}
	// Nothing validates against it yet.
	if s.themeKinds.Known("spring") {
		t.Error("an unapproved kind is in the registry")
	}

	// ⚠️ The handshake SUCCEEDED, and said which token it held back. A hard
	// refusal would mean a frontend adding one experimental token cannot connect
	// at all; silence would mean discovering it as a rendering bug.
	if got := toStrings(out["pendingKinds"]); len(got) != 1 || got[0] != "spring" {
		t.Errorf("pendingKinds = %v, want [spring]", out["pendingKinds"])
	}
	if got := toStrings(out["deferredTokens"]); len(got) != 1 || got[0] != "--motion" {
		t.Errorf("deferredTokens = %v, want [--motion]", out["deferredTokens"])
	}
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if _, ok := fam.TokenByName("--motion"); ok {
		t.Error("a token whose kind nobody approved was added to the family; nothing would validate its values")
	}
	if _, ok := fam.TokenByName("--primary"); !ok {
		t.Error("one deferred token took the rest of the manifest down with it")
	}

	// The operator reads the pattern and agrees.
	rec := adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/spring", `{"approved":true}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	// ⚠️ In force immediately — the reload IS the change. Without it the console
	// reports success and the deployment still validates against what it loaded
	// at boot.
	if !s.themeKinds.Known("spring") {
		t.Fatal("an approved kind is not in the running registry; approval did nothing until a restart")
	}
	if err := s.themeKinds.Validate("spring", "1.2 0.4"); err != nil {
		t.Errorf("the approved pattern does not accept its own example: %v", err)
	}
	if err := s.themeKinds.Validate("spring", "not a spring"); err == nil {
		t.Error("the approved kind accepts anything")
	}

	// The next handshake lands the token that was waiting.
	_, out = handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{
		"kinds":[{"name":"spring","pattern":"[0-9.]+ [0-9.]+","description":"刚度 和 阻尼"}],
		"tokens":[{"name":"--motion","kind":"spring"},{"name":"--primary","kind":"color"}]}}`)
	fam, _ = s.store.Frontends().GetFamily(ctx, "liuli")
	if _, ok := fam.TokenByName("--motion"); !ok {
		t.Error("the token is still held back after its kind was approved")
	}
	if got := toStrings(out["pendingKinds"]); len(got) != 0 {
		t.Errorf("pendingKinds is still %v after approval", got)
	}
}

// ⚠️ Approval cannot outlive the text it was granted against.
//
// Otherwise the gate is one handshake away from decorative: a frontend gets
// `^[0-9]+$` approved, ships `.*` next week, and the deployment runs whatever
// arrived later with the flag still set.
func TestChangingThePatternRevokesTheApproval(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	propose := func(pattern string) {
		t.Helper()
		handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[
			{"name":"spring","pattern":"`+pattern+`","description":"d"}],
			"tokens":[{"name":"--primary","kind":"color"}]}}`)
	}
	propose(`[0-9]+`)
	adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/spring", `{"approved":true}`, withRootHeader(s))
	if !s.themeKinds.Known("spring") {
		t.Fatal("setup: the approval did not take")
	}

	// Same text again — an ordinary page load. It must NOT disturb the
	// approval, or every reload would undo the operator's decision.
	propose(`[0-9]+`)
	if k, _ := s.store.ThemeKinds().Get(ctx, "spring"); !k.Approved {
		t.Error("re-proposing the SAME pattern revoked the approval; every page load would undo it")
	}

	// Different text. The approval goes, and the registry stops accepting the
	// old pattern's values right away.
	propose(`.*`)
	k, _ := s.store.ThemeKinds().Get(ctx, "spring")
	if k.Approved {
		t.Fatal("a frontend changed its pattern and kept the approval it was granted for another one")
	}
	if k.Pattern != `.*` {
		t.Errorf("the new pattern was not stored: %q", k.Pattern)
	}
	if s.themeKinds.Known("spring") {
		t.Error("the revoked kind is still in the running registry; the deployment validates against text nobody approved")
	}
}

// A frontend may not propose a name that already means something.
func TestAFrontendCannotProposeANameThatMeansSomething(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	for _, name := range []string{"color", "length", "ratio", "one-of[a,b]", "list-of<color>", "Spring", "sp ring", ""} {
		handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[
			{"name":"`+name+`","pattern":".*"}],"tokens":[{"name":"--primary","kind":"color"}]}}`)
		if _, err := s.store.ThemeKinds().Get(ctx, name); err == nil {
			t.Errorf("a frontend proposed %q and it was recorded", name)
		}
	}
	// ⚠️ …and `color` still means what it always meant. A frontend proposing it
	// and an operator approving without reading closely would widen validation
	// for every theme on the deployment, retroactively.
	if err := s.themeKinds.Validate("color", "not-a-color"); err == nil {
		t.Error("the embedded color kind was widened by a proposal")
	}
}

// The operator may not redefine a primitive from a web form either.
//
// They MAY through THEME_KINDS_DIR — a file on the machine they already have.
// The difference matters: a stolen console session is not the same as shell
// access, and one request that retroactively widens `color` for every stored
// theme is exactly the thing to keep behind the harder door.
func TestThePrimitivesCannotBeRedefinedFromTheConsole(t *testing.T) {
	s := adminServer(t)
	for _, name := range []string{"color", "duration", "one-of[a,b]"} {
		rec := adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/"+name, `{"pattern":".*","approved":true}`, withRootHeader(s))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s answered %d, want 400", name, rec.Code)
		}
	}
	// A file on disk still can — the layer below is unchanged by any of this.
	if errs := s.themeKinds.Merge([]theme.Kind{{Name: "color", Pattern: `zzz`}}, theme.OriginFile); len(errs) > 0 {
		t.Fatalf("the file layer can no longer redefine a primitive: %v", errs)
	}
	if err := s.themeKinds.Validate("color", "zzz"); err != nil {
		t.Errorf("the file layer's redefinition is not in force: %v", err)
	}
}

// A pattern that will not compile never reaches an operator's screen, and never
// gets stored by the console either.
func TestAPatternThatCannotCompileIsRefusedOnBothPaths(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[
		{"name":"broken","pattern":"([a-z"}],"tokens":[{"name":"--primary","kind":"color"}]}}`)
	if _, err := s.store.ThemeKinds().Get(ctx, "broken"); err == nil {
		t.Error("a pattern that cannot compile was queued for an operator to approve")
	}
	rec := adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/broken", `{"pattern":"([a-z"}`, withRootHeader(s))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("the console stored an uncompilable pattern: %d %s", rec.Code, rec.Body)
	}
	// Too long, too — RE2 does not backtrack, but its program still runs against
	// every theme value on every write.
	long := strings.Repeat("a", theme.MaxStoredPattern+10)
	rec = adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/huge", `{"pattern":"`+long+`"}`, withRootHeader(s))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an over-long pattern was accepted: %d", rec.Code)
	}
}

// ⚠️ The proposal queue is bounded, because the handshake is unauthenticated.
func TestProposalsAreCappedBecauseAnybodyCanPropose(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()

	// ⚠️ ACROSS REQUESTS, which is the shape that actually matters. A single
	// oversized request is refused by the array bound before any work is done
	// (see TestAHugeProposalListIsRefusedBeforeAnyWorkIsDone); an attacker
	// simply loops instead, and the ROW cap is the thing that stops that.
	//
	// The first version of this test sent one huge list, which the array bound
	// then refused outright — so it was measuring the wrong ceiling. Its own
	// "nothing was recorded at all" guard is what caught that.
	sent := 0
	for req := 0; req < 5; req++ {
		var b strings.Builder
		for i := 0; i < domain.MaxProposedKinds/2; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"name":"k` + itoaTest(sent) + `","pattern":"[0-9]+"}`)
			sent++
		}
		rec, _ := handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[`+b.String()+`],
			"tokens":[{"name":"--primary","kind":"color"}]}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d was refused: %d %s", req, rec.Code, rec.Body)
		}
	}
	if sent <= domain.MaxProposedKinds {
		t.Fatalf("the test only offered %d proposals; the cap is %d and would never be reached", sent, domain.MaxProposedKinds)
	}

	n, err := s.store.ThemeKinds().CountPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n > domain.MaxProposedKinds {
		t.Errorf("%d pending proposals after %d offered from unauthenticated requests; the cap is %d",
			n, sent, domain.MaxProposedKinds)
	}
	if n == 0 {
		t.Fatal("nothing was recorded at all — this test would pass for the wrong reason")
	}
	// …and an operator approving one frees a slot, or the queue jams forever
	// after the first sixty-four proposals anybody ever made.
	rows, _ := s.store.ThemeKinds().List(ctx)
	rows[0].Approved = true
	if err := s.store.ThemeKinds().Upsert(ctx, rows[0]); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.store.ThemeKinds().CountPending(ctx); after != n-1 {
		t.Errorf("approving one left %d pending, want %d — the queue never drains", after, n-1)
	}
}

// Approved-but-not-in-force is a real state and the screen must be able to show
// it.
func TestApprovedIsNotTheSameAsInForce(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	// A row that is approved and whose pattern the loader will skip. Written
	// straight to the store, because neither the handshake nor the console will
	// produce it — which is the point: it arrives by a pattern that stopped
	// compiling under a later build, or by a hand-edited row.
	if err := s.store.ThemeKinds().Upsert(ctx, domain.ThemeKind{
		Name: "wonky", Pattern: `([a-z`, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadThemeKinds(ctx); err != nil {
		t.Fatal(err)
	}
	rec := adminReq(t, s, http.MethodGet, "/api/admin/theme-kinds", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Kinds []struct {
			Name     string `json:"name"`
			Approved bool   `json:"approved"`
			InForce  bool   `json:"inForce"`
			Shadows  string `json:"shadows"`
		} `json:"kinds"`
		MaxPending int `json:"maxPending"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Kinds) != 1 {
		t.Fatalf("got %d rows", len(body.Kinds))
	}
	if !body.Kinds[0].Approved {
		t.Error("the row lost its approved flag")
	}
	if body.Kinds[0].InForce {
		t.Error("a row whose pattern the loader skipped is reported as in force — the one state an operator most needs to see, hidden")
	}
	if body.MaxPending != domain.MaxProposedKinds {
		t.Errorf("maxPending is %d, want %d — the console cannot state the cap correctly", body.MaxPending, domain.MaxProposedKinds)
	}

	// A row that shadows a lower layer says so.
	if err := s.store.ThemeKinds().Upsert(ctx, domain.ThemeKind{Name: "paper", Pattern: `[0-9]+`, Approved: true}); err != nil {
		t.Fatal(err)
	}
	s.themeKinds.Merge([]theme.Kind{{Name: "paper", Pattern: `[a-z]+`}}, theme.OriginFile)
	if err := s.ReloadThemeKinds(ctx); err != nil {
		t.Fatal(err)
	}
	rec = adminReq(t, s, http.MethodGet, "/api/admin/theme-kinds", "", withRootHeader(s))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range body.Kinds {
		if k.Name != "paper" {
			continue
		}
		if k.Shadows != theme.OriginFile {
			t.Errorf("paper reports shadows=%q; an operator cannot tell adding from redefining", k.Shadows)
		}
	}
}

// Revoking from the console takes effect on the next write, not on the next
// restart.
func TestRevokingFromTheConsoleTakesEffectImmediately(t *testing.T) {
	s, sid := newAgentTestServer(t)
	s.cookies = auth.NewCookieSigner("kind-test-secret")
	s.cfg.AdminToken = "the-real-token"
	ctx := context.Background()
	if err := s.store.ThemeKinds().Upsert(ctx, domain.ThemeKind{
		Name: "grain", Pattern: `[0-9]+`, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadThemeKinds(ctx); err != nil {
		t.Fatal(err)
	}
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[
		{"name":"--grain","kind":"grain"}]}}`)
	post := func(body string) int {
		req := httptestNewJSON(http.MethodPost, "/api/themes", body)
		req.Header.Set("X-Session-Token", s.cookies.Sign(sid))
		req.Header.Set(frontendBuildHeader, "web-1")
		rec := httptestRecord(s, req)
		return rec.Code
	}
	if code := post(`{"name":"t","variables":{"--grain":"42"}}`); code != http.StatusOK {
		t.Fatalf("setup: a theme using the approved kind was refused: %d", code)
	}

	rec := adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/grain", `{"approved":false}`, func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	// ⚠️ The write path must refuse NOW. Merge is additive, so without the
	// registry's separate database layer this would keep accepting values until
	// somebody redeployed.
	if code := post(`{"name":"t2","variables":{"--grain":"43"}}`); code != http.StatusBadRequest {
		t.Errorf("a revoked kind still validates theme writes: %d", code)
	}
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func itoaTest(i int) string { return strconv.Itoa(i) }

func httptestNewJSON(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, versionPath(path), strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func httptestRecord(s *Server, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}

// ⚠️ An over-long pattern is REFUSED, never truncated — on both paths.
//
// cleanManifestText cuts a string to length, which is right for prose: a
// description clipped at 200 characters is still a description. It is wrong for
// EXECUTABLE text. A regex cut at 512 characters is a different regex, and some
// truncations still compile and mean something else entirely — so the operator
// would be reading and approving one pattern while a different one validated
// every write.
//
// The console path had exactly this bug; it was caught by a test that asserted
// on the outcome rather than on the call.
func TestAnOverLongPatternIsRefusedRatherThanTruncated(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	// A pattern that is a different, still-compilable regex once truncated:
	// the alternation's tail is cut off, leaving a shorter but valid one.
	long := "aaa|" + strings.Repeat("b", theme.MaxStoredPattern) + "|ccc"

	rec := adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/wide", `{"pattern":`+jsonStr(long)+`}`, withRootHeader(s))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("the console answered %d for an over-long pattern, want 400", rec.Code)
	}
	if _, err := s.store.ThemeKinds().Get(ctx, "wide"); err == nil {
		got, _ := s.store.ThemeKinds().Get(ctx, "wide")
		t.Errorf("a truncated pattern was stored: %q", got.Pattern)
	}

	handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[
		{"name":"wide2","pattern":`+jsonStr(long)+`}],"tokens":[{"name":"--primary","kind":"color"}]}}`)
	if got, err := s.store.ThemeKinds().Get(ctx, "wide2"); err == nil {
		t.Errorf("the handshake stored a truncated pattern: %q (%d chars)", got.Pattern, len(got.Pattern))
	}

	// …and one at exactly the limit still works, or the bound is a wall.
	ok := strings.Repeat("b", theme.MaxStoredPattern)
	rec = adminReq(t, s, http.MethodPut, "/api/admin/theme-kinds/edge", `{"pattern":`+jsonStr(ok)+`}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Errorf("a pattern at exactly the limit was refused: %d %s", rec.Code, rec.Body)
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ⚠️ The proposal ARRAY is bounded, not just the row count.
//
// MaxProposedKinds caps how many rows may exist; it does not cap WORK. Each
// proposal costs a regex compile and a database read before the row cap can
// refuse it, so a hundred thousand of them in one unauthenticated request is a
// hundred thousand compiles and reads — the ceiling holding perfectly while the
// request runs for a minute. A cap on rows is not a cap on cost.
func TestAHugeProposalListIsRefusedBeforeAnyWorkIsDone(t *testing.T) {
	s := adminServer(t)
	var b strings.Builder
	for i := 0; i < domain.MaxProposedKinds*4; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"k` + itoaTest(i) + `","pattern":"[0-9]+"}`)
	}
	rec, _ := handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[`+b.String()+`],"tokens":[]}}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a %d-kind proposal list answered %d, want 400", domain.MaxProposedKinds*4, rec.Code)
	}
	// Nothing was written — the refusal is before the loop, not inside it.
	if n, _ := s.store.ThemeKinds().CountPending(context.Background()); n != 0 {
		t.Errorf("%d rows were written before the request was refused", n)
	}
	// …and a list at exactly the cap still works, or the bound is a wall.
	b.Reset()
	for i := 0; i < domain.MaxProposedKinds; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"ok` + itoaTest(i) + `","pattern":"[0-9]+"}`)
	}
	rec, _ = handshake(t, s, `{"familyId":"liuli","theme":{"kinds":[`+b.String()+`],"tokens":[]}}`)
	if rec.Code != http.StatusOK {
		t.Errorf("a list at exactly the cap was refused: %d %s", rec.Code, rec.Body)
	}
}
