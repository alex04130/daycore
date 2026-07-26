package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/auth"
	"daycore/internal/domain"
)

// POST /api/auth/register — email + password sign-up.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.authRateLimit(w, r) {
		return
	}
	var body struct {
		Email       string  `json:"email"`
		Password    string  `json:"password"`
		Name        *string `json:"name"`
		TokenInBody bool    `json:"tokenInBody"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || !strings.Contains(email, "@") {
		s.writeErr(w, http.StatusBadRequest, "invalid_email", "请输入有效的邮箱")
		return
	}
	if len(body.Password) < 8 {
		s.writeErr(w, http.StatusBadRequest, "weak_password", "密码至少 8 位")
		return
	}
	ctx := r.Context()
	if _, err := s.store.Users().GetByEmail(ctx, email); err == nil {
		s.writeErr(w, http.StatusConflict, "email_taken", "该邮箱已注册")
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}

	hash, err := s.hasher.Hash(body.Password)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}
	user, err := s.store.Users().Upsert(ctx, &domain.User{Email: &email, Name: body.Name})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}
	if err := s.store.Auth().UpsertCredential(ctx, &domain.Credential{UserID: user.ID, PasswordHash: hash}); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}
	s.finishLogin(w, r, user, body.TokenInBody)
}

// POST /api/auth/login — email + password sign-in.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authRateLimit(w, r) {
		return
	}
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		TokenInBody bool   `json:"tokenInBody"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	ctx := r.Context()

	user, err := s.store.Users().GetByEmail(ctx, email)
	if err != nil {
		s.writeErr(w, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	cred, err := s.store.Auth().GetCredentialByUserID(ctx, user.ID)
	if err != nil {
		s.writeErr(w, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	ok, err := s.hasher.Verify(body.Password, cred.PasswordHash)
	if err != nil || !ok {
		s.writeErr(w, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	s.finishLogin(w, r, user, body.TokenInBody)
}

// POST /api/auth/logout — clear the auth cookie and revoke all issued JWTs by
// bumping the user's token version.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if uid := userIDFrom(r.Context()); uid != "" {
		_ = s.store.Users().IncrementTokenVersion(r.Context(), uid)
	}
	s.clearAuthCookie(w)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/me — the current user, or {user:null}.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	if uid == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	user, err := s.store.Users().GetByID(r.Context(), uid)
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, user *domain.User, tokenInBody bool) {
	token, err := s.issueAndLink(w, r, user)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "登录失败")
		return
	}
	resp := map[string]any{"ok": true, "user": user}
	if tokenInBody {
		// Native clients keep the JWT themselves (no cookie jar); logout still
		// revokes it everywhere via the token-version bump.
		resp["token"] = token
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// issueAndLink sets the auth cookie, returns the issued JWT, and claims the
// current session for the user — if the user already has a canonical data
// session, anonymous data is merged into it; otherwise the current sid becomes
// the canonical session.
func (s *Server) issueAndLink(w http.ResponseWriter, r *http.Request, user *domain.User) (string, error) {
	token, err := s.tokens.Issue(user.ID, user.TokenVersion)
	if err != nil {
		return "", err
	}
	s.setAuthCookie(w, token)
	sid := sessionIDFrom(r.Context())
	ctx := r.Context()
	if sid == "" {
		// Bearer-only clients (native apps) carry no anonymous session. A user
		// without a canonical data session would then hit no_session on every
		// data endpoint — mint the canonical session on the spot.
		if user.DataSessionID == "" {
			newID, err := auth.NewSessionID()
			if err != nil {
				return "", err
			}
			if _, err := s.store.Sessions().GetOrCreate(ctx, newID); err != nil {
				return "", err
			}
			if err := s.store.Users().SetDataSession(ctx, user.ID, newID); err != nil {
				return "", err
			}
			_, _ = s.store.Sessions().Update(ctx, newID, domain.SessionUpdate{UserID: &user.ID})
			user.DataSessionID = newID
		}
		return token, nil
	}

	// Fast path: user has no canonical data session yet — adopt current sid.
	if user.DataSessionID == "" {
		_ = s.store.Users().SetDataSession(ctx, user.ID, sid)
		_, _ = s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{UserID: &user.ID})
		return token, nil
	}
	if user.DataSessionID == sid {
		_, _ = s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{UserID: &user.ID})
		return token, nil
	}

	// Conflict: user already has a canonical data session from another
	// device. Merge the current anonymous session's data in.
	_ = s.mergeSessionData(ctx, sid, user.DataSessionID)
	_, _ = s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{UserID: &user.ID})
	return token, nil
}

// mergeSessionData folds the anonymous session's data into the canonical
// session. Best-effort: errors are logged but never returned — partial merges
// are acceptable (the anonymous session data is still reachable).
func (s *Server) mergeSessionData(ctx context.Context, anonSID, canonSID string) error {
	// day_plans: merge by date; canon blocks take priority, anon non-rule blocks append.
	anonPlans, _ := s.store.DayPlans().Range(ctx, anonSID, "1970-01-01", "2099-12-31")
	for _, ap := range anonPlans {
		canonPlan, err := s.store.DayPlans().Get(ctx, canonSID, ap.Date)
		if errors.Is(err, domain.ErrNotFound) {
			ap.SessionID = canonSID
			_, _ = s.store.DayPlans().Upsert(ctx, &ap)
			continue
		}
		if err != nil {
			continue
		}
		for _, b := range ap.Blocks {
			if b.Origin != domain.OriginRule {
				canonPlan.Blocks = append(canonPlan.Blocks, b)
			}
		}
		canonPlan.SessionID = canonSID
		_, _ = s.store.DayPlans().Upsert(ctx, canonPlan)
	}

	// rules: dedupe by (title, kind, freq, interval, time, by_weekday, date).
	anonRules, _ := s.store.Rules().List(ctx, anonSID)
	canonRules, _ := s.store.Rules().List(ctx, canonSID)
	seen := map[string]bool{}
	for _, r := range canonRules {
		seen[ruleMergeKey(r)] = true
	}
	for _, r := range anonRules {
		if !seen[ruleMergeKey(r)] {
			r.SessionID = canonSID
			if _, err := s.store.Rules().Create(ctx, &r); err != nil {
				s.log.Warn("merge rule", "err", err)
			}
		}
	}

	// memory: dedupe by fact text.
	anonFacts, _ := s.store.Memory().ListFacts(ctx, anonSID)
	canonFacts, _ := s.store.Memory().ListFacts(ctx, canonSID)
	seenFacts := map[string]bool{}
	for _, f := range canonFacts {
		seenFacts[f.Fact] = true
	}
	for _, f := range anonFacts {
		if !seenFacts[f.Fact] {
			_, _ = s.store.Memory().AddFact(ctx, &domain.MemoryFact{SessionID: canonSID, Fact: f.Fact, Source: f.Source, Type: f.Type})
		}
	}

	// moods / imports: simple append.
	anonMoods, _ := s.store.Moods().List(ctx, anonSID, 200)
	for _, m := range anonMoods {
		m.SessionID = canonSID
		_, _ = s.store.Moods().Create(ctx, &m)
	}
	anonImports, _ := s.store.Memory().ListImports(ctx, anonSID, 200)
	for _, imp := range anonImports {
		imp.SessionID = canonSID
		_, _ = s.store.Memory().AddImport(ctx, &imp)
	}

	// materials: append with fresh ids.
	anonMaterials, _ := s.store.Materials().List(ctx, anonSID, "", "", 0, 0)
	for i := range anonMaterials {
		m := anonMaterials[i]
		m.ID = ""
		m.SessionID = canonSID
		_, _ = s.store.Materials().Create(ctx, &m)
	}

	// wishes: append with fresh ids.
	anonWishes, _ := s.store.Wishes().List(ctx, anonSID, "")
	for i := range anonWishes {
		wsh := anonWishes[i]
		wsh.ID = ""
		wsh.SessionID = canonSID
		_, _ = s.store.Wishes().Create(ctx, &wsh)
	}

	// custom themes: append with fresh ids.
	anonThemes, _ := s.store.Themes().List(ctx, anonSID)
	for i := range anonThemes {
		t := anonThemes[i]
		t.ID = ""
		t.SessionID = canonSID
		_, _ = s.store.Themes().Create(ctx, &t)
	}

	// canvas courses & assignments: upsert by canvas id (dedupes shared courses).
	anonCourses, _ := s.store.Courses().List(ctx, anonSID)
	for i := range anonCourses {
		c := anonCourses[i]
		c.SessionID = canonSID
		_, _ = s.store.Courses().UpsertByCanvasID(ctx, &c)
	}
	anonAssignments, _ := s.store.Assignments().List(ctx, anonSID, domain.AssignmentFilter{})
	for i := range anonAssignments {
		a := anonAssignments[i]
		a.SessionID = canonSID
		_, _ = s.store.Assignments().UpsertByCanvasID(ctx, &a)
	}

	// chat threads + their messages: recreate under the canonical session.
	anonThreads, _ := s.store.Chats().ListThreads(ctx, anonSID)
	for i := range anonThreads {
		th := anonThreads[i]
		msgs, _ := s.store.Chats().ListMessages(ctx, th.ID, anonSID, "", 100000)
		th.ID = ""
		th.SessionID = canonSID
		newThread, err := s.store.Chats().CreateThread(ctx, &th)
		if err != nil || newThread == nil {
			continue
		}
		// ListMessages returns newest-first; re-append chronologically.
		out := make([]domain.ChatMessage, 0, len(msgs))
		for j := len(msgs) - 1; j >= 0; j-- {
			m := msgs[j]
			m.ID = ""
			m.ThreadID = newThread.ID
			m.SessionID = canonSID
			out = append(out, m)
		}
		if len(out) > 0 {
			_ = s.store.Chats().AppendMessages(ctx, out)
		}
	}

	// companion memory (legacy history): only if the canonical session has none.
	if cm, err := s.store.Companion().Get(ctx, anonSID); err == nil && cm != nil && len(cm.ConversationHistory) > 0 {
		if ccm, _ := s.store.Companion().Get(ctx, canonSID); ccm == nil || len(ccm.ConversationHistory) == 0 {
			_ = s.store.Companion().Upsert(ctx, canonSID, cm.ConversationHistory, cm.KeyFacts)
		}
	}

	// session settings: adopt the anon persona/language only where canon is unset.
	if anonSess, err := s.store.Sessions().Get(ctx, anonSID); err == nil && anonSess != nil {
		if canonSess, err := s.store.Sessions().Get(ctx, canonSID); err == nil && canonSess != nil {
			var upd domain.SessionUpdate
			changed := false
			if canonSess.PersonaPrompt == "" && anonSess.PersonaPrompt != "" {
				upd.PersonaPrompt = &anonSess.PersonaPrompt
				changed = true
			}
			if canonSess.Language == "" && anonSess.Language != "" {
				upd.Language = &anonSess.Language
				changed = true
			}
			if changed {
				_, _ = s.store.Sessions().Update(ctx, canonSID, upd)
			}
		}
	}

	// Intentionally NOT merged: feedback_logs (write-only, no list API) and
	// temp_context (short-lived TTL data). channel_bindings are left alone too —
	// an external account maps to exactly one session, so merging could create a
	// conflicting double-binding; re-binding after login is the safe path.

	return nil
}

func ruleMergeKey(r domain.ScheduleRule) string {
	tm := ""
	if r.Time != nil {
		tm = *r.Time
	}
	dt := ""
	if r.Date != nil {
		dt = *r.Date
	}
	wd := ""
	for _, d := range r.ByWeekday {
		wd += strconv.Itoa(d) + ","
	}
	// Full identity — two rules with the same title/kind/freq but a different
	// time, weekday set, interval, or date are NOT duplicates.
	return strings.Join([]string{r.Title, r.Kind, r.Freq, strconv.Itoa(r.Interval), tm, wd, dt}, "|")
}
