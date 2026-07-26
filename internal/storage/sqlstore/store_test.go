package sqlstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"daycore/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(sqliteDialect{}, "file:"+t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func TestSQLStoreRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// session get-or-create + update + increment
	sess, err := s.Sessions().GetOrCreate(ctx, "sid1")
	if err != nil || sess.AssistantName != "Leo" {
		t.Fatalf("session: %+v err=%v", sess, err)
	}
	if err := s.Sessions().IncrementInteraction(ctx, "sid1"); err != nil {
		t.Fatal(err)
	}
	name := "小艾"
	sess, _ = s.Sessions().Update(ctx, "sid1", domain.SessionUpdate{AssistantName: &name})
	if sess.AssistantName != "小艾" || sess.InteractionCount != 1 {
		t.Fatalf("update: %+v", sess)
	}

	// day plan upsert + get + range (JSON blocks survive the round-trip)
	note := "加油"
	_, err = s.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: "sid1", Date: "2026-06-21", Note: &note,
		Blocks: []domain.TimeBlock{{ID: "b1", Title: "x", Type: domain.BlockTask, TimeMode: domain.TimeFloating}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.DayPlans().Get(ctx, "sid1", "2026-06-21")
	if err != nil || len(got.Blocks) != 1 || got.Blocks[0].ID != "b1" {
		t.Fatalf("plan get: %+v err=%v", got, err)
	}
	rng, _ := s.DayPlans().Range(ctx, "sid1", "2026-06-01", "2026-06-30")
	if len(rng) != 1 {
		t.Fatalf("range len=%d", len(rng))
	}

	// mood
	if _, err := s.Moods().Create(ctx, &domain.MoodCheckin{SessionID: "sid1", Mood: "great"}); err != nil {
		t.Fatal(err)
	}
	if moods, _ := s.Moods().List(ctx, "sid1", 10); len(moods) != 1 {
		t.Fatalf("mood list len=%d", len(moods))
	}

	// user + credential + oauth
	email := "x@y.com"
	u, err := s.Users().Upsert(ctx, &domain.User{Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Auth().UpsertCredential(ctx, &domain.Credential{UserID: u.ID, PasswordHash: "$argon2id$..."}); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Auth().GetCredentialByUserID(ctx, u.ID); err != nil || c.PasswordHash == "" {
		t.Fatalf("credential: %+v err=%v", c, err)
	}
	if err := s.Auth().CreateOAuthIdentity(ctx, &domain.OAuthIdentity{UserID: u.ID, Provider: "google", ProviderUserID: "g1"}); err != nil {
		t.Fatal(err)
	}
	if oi, err := s.Auth().GetOAuthIdentity(ctx, "google", "g1"); err != nil || oi.UserID != u.ID {
		t.Fatalf("oauth identity: %+v err=%v", oi, err)
	}

	// companion memory + prompts
	if err := s.Companion().Upsert(ctx, "sid1", []domain.Message{{Role: domain.RoleUser, Content: "hi"}}, []string{"likes tea"}); err != nil {
		t.Fatal(err)
	}
	if mem, err := s.Companion().Get(ctx, "sid1"); err != nil || len(mem.ConversationHistory) != 1 {
		t.Fatalf("companion: %+v err=%v", mem, err)
	}
	if err := s.Prompts().Set(ctx, "day_plan_text", "zh-CN", "override"); err != nil {
		t.Fatal(err)
	}
	if p, err := s.Prompts().Get(ctx, "day_plan_text", "zh-CN"); err != nil || p.Content != "override" {
		t.Fatalf("prompt: %+v err=%v", p, err)
	}
	if _, err := s.Prompts().Get(ctx, "day_plan_text", "en-US"); err != domain.ErrNotFound {
		t.Fatalf("prompt other locale should be ErrNotFound, got %v", err)
	}

	// not-found path
	if _, err := s.DayPlans().Get(ctx, "sid1", "1999-01-01"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionLanguageAndImportToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Sessions().GetOrCreate(ctx, "sid1"); err != nil {
		t.Fatal(err)
	}
	lang, token := "en-US", "tok-abc123"
	sess, err := s.Sessions().Update(ctx, "sid1", domain.SessionUpdate{Language: &lang, ImportToken: &token})
	if err != nil || sess.Language != "en-US" || sess.ImportToken != "tok-abc123" {
		t.Fatalf("update: %+v err=%v", sess, err)
	}
	byTok, err := s.Sessions().GetByImportToken(ctx, "tok-abc123")
	if err != nil || byTok.ID != "sid1" {
		t.Fatalf("by token: %+v err=%v", byTok, err)
	}
	if _, err := s.Sessions().GetByImportToken(ctx, "nope"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := s.Sessions().GetByImportToken(ctx, ""); err != domain.ErrNotFound {
		t.Fatalf("empty token must never match, got %v", err)
	}
}

func TestRulesCoursesAssignments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// rules CRUD
	tm := "09:00"
	rule, err := s.Rules().Create(ctx, &domain.ScheduleRule{
		SessionID: "sid1", Title: "浇花", Type: domain.BlockTask, Time: &tm,
		Timezone: "Asia/Shanghai", TimeMode: domain.TimeFloating,
		Kind: domain.RuleRecurring, Freq: domain.FreqEveryNDays, Interval: 3,
		StartDate: "2026-07-06", Active: true, Source: "chat",
	})
	if err != nil || rule.ID == "" || rule.Interval != 3 {
		t.Fatalf("rule create: %+v err=%v", rule, err)
	}
	n := 5
	rule, err = s.Rules().Update(ctx, "sid1", rule.ID, domain.ScheduleRuleUpdate{Interval: &n})
	if err != nil || rule.Interval != 5 {
		t.Fatalf("rule update: %+v err=%v", rule, err)
	}
	if _, err := s.Rules().Update(ctx, "other", rule.ID, domain.ScheduleRuleUpdate{Interval: &n}); err != domain.ErrNotFound {
		t.Fatalf("cross-session update must be ErrNotFound, got %v", err)
	}
	if rules, _ := s.Rules().List(ctx, "sid1"); len(rules) != 1 {
		t.Fatalf("rule list")
	}
	if err := s.Rules().Delete(ctx, "sid1", rule.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Rules().Delete(ctx, "sid1", rule.ID); err != domain.ErrNotFound {
		t.Fatalf("double delete: %v", err)
	}

	// courses upsert refresh
	score := 92.5
	c, err := s.Courses().UpsertByCanvasID(ctx, &domain.Course{
		SessionID: "sid1", CanvasID: "c100", Name: "软件设计", CurrentScore: &score,
	})
	if err != nil || c.ID == "" {
		t.Fatalf("course: %+v err=%v", c, err)
	}
	score2 := 88.0
	c2, err := s.Courses().UpsertByCanvasID(ctx, &domain.Course{
		SessionID: "sid1", CanvasID: "c100", Name: "软件设计", CurrentScore: &score2,
	})
	if err != nil || c2.ID != c.ID || *c2.CurrentScore != 88.0 {
		t.Fatalf("course refresh: %+v err=%v", c2, err)
	}

	// assignments upsert preserves local status
	due := fromMillis(nowMillis() + 48*3600*1000)
	a, err := s.Assignments().UpsertByCanvasID(ctx, &domain.Assignment{
		SessionID: "sid1", CourseID: c.ID, CanvasID: "a1", Title: "作业1", DueAt: &due, Source: "canvas",
	})
	if err != nil || a.Status != domain.AssignmentPending {
		t.Fatalf("assignment: %+v err=%v", a, err)
	}
	if err := s.Assignments().SetStatus(ctx, "sid1", a.ID, domain.AssignmentPlanned); err != nil {
		t.Fatal(err)
	}
	a2, err := s.Assignments().UpsertByCanvasID(ctx, &domain.Assignment{
		SessionID: "sid1", CourseID: c.ID, CanvasID: "a1", Title: "作业1（改）", DueAt: &due, Source: "canvas",
	})
	if err != nil || a2.Status != domain.AssignmentPlanned || a2.Title != "作业1（改）" {
		t.Fatalf("assignment refresh must keep status: %+v err=%v", a2, err)
	}
	from := fromMillis(nowMillis())
	to := fromMillis(nowMillis() + 7*24*3600*1000)
	items, err := s.Assignments().List(ctx, "sid1", domain.AssignmentFilter{DueFrom: &from, DueTo: &to})
	if err != nil || len(items) != 1 {
		t.Fatalf("assignment list: %d err=%v", len(items), err)
	}
}

func TestThemesRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.Themes().Create(ctx, &domain.CustomTheme{
		SessionID: "sid1", Name: "蜜桃汽水", Base: "sunset", Dark: false,
		Variables: map[string]string{"--primary": "#f472b6", "--bg-start": "#fdf2f8"},
	})
	if err != nil || created.ID == "" || created.Variables["--primary"] != "#f472b6" {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	dark := true
	vars := map[string]string{"--primary": "#a78bfa"}
	updated, err := s.Themes().Update(ctx, "sid1", created.ID, domain.CustomThemeUpdate{Dark: &dark, Variables: &vars})
	if err != nil || !updated.Dark || updated.Variables["--primary"] != "#a78bfa" || len(updated.Variables) != 1 {
		t.Fatalf("update: %+v err=%v", updated, err)
	}
	if _, err := s.Themes().Update(ctx, "other", created.ID, domain.CustomThemeUpdate{Dark: &dark}); err != domain.ErrNotFound {
		t.Fatalf("cross-session update must be ErrNotFound, got %v", err)
	}
	if list, _ := s.Themes().List(ctx, "sid1"); len(list) != 1 {
		t.Fatalf("list")
	}
	if err := s.Themes().Delete(ctx, "sid1", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Themes().Delete(ctx, "sid1", created.ID); err != domain.ErrNotFound {
		t.Fatalf("double delete: %v", err)
	}
}

func TestOperationLogs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first := &domain.OperationLog{
		SessionID: "sid1", Actor: domain.ActorUser, Action: "plan_upsert",
		TargetID: "b1", Date: "2026-07-12", Summary: "first",
		Detail: `{"before":null,"after":[]}`, Status: domain.OpStatusOK, RequestID: "req-1",
	}
	if err := s.OpLogs().Add(ctx, first); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // distinct created_at millis for ordering
	long := strings.Repeat("长", domain.OpLogSummaryLimit+100)
	if err := s.OpLogs().Add(ctx, &domain.OperationLog{
		SessionID: "sid1", Actor: domain.ActorAgent, Action: "plan_autoplan", Summary: long,
	}); err != nil {
		t.Fatal(err)
	}

	logs, err := s.OpLogs().List(ctx, "sid1", 10)
	if err != nil || len(logs) != 2 {
		t.Fatalf("list: len=%d err=%v", len(logs), err)
	}
	if logs[0].Action != "plan_autoplan" || logs[1].Action != "plan_upsert" {
		t.Fatalf("want newest first, got %s, %s", logs[0].Action, logs[1].Action)
	}
	if n := len([]rune(logs[0].Summary)); n != domain.OpLogSummaryLimit {
		t.Fatalf("summary must be truncated to %d runes, got %d", domain.OpLogSummaryLimit, n)
	}
	got := logs[1]
	if got.Actor != domain.ActorUser || got.TargetID != "b1" || got.Date != "2026-07-12" ||
		got.Detail != `{"before":null,"after":[]}` || got.Status != domain.OpStatusOK || got.RequestID != "req-1" {
		t.Fatalf("round-trip: %+v", got)
	}
	if short, _ := s.OpLogs().List(ctx, "sid1", 1); len(short) != 1 {
		t.Fatalf("limit: len=%d", len(short))
	}
	if other, _ := s.OpLogs().List(ctx, "other", 10); len(other) != 0 {
		t.Fatalf("foreign session must see nothing, len=%d", len(other))
	}
}

func TestMigrateIdempotentAndLegacyPromptCopy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed the legacy prompts table, re-run Migrate, expect a zh-CN backfill.
	if _, err := s.exec(ctx, `INSERT INTO prompts (prompt_key, content, updated_at) VALUES (?, ?, ?)`,
		"companion", "legacy-zh", nowMillis()); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	p, err := s.Prompts().Get(ctx, "companion", "zh-CN")
	if err != nil || p.Content != "legacy-zh" {
		t.Fatalf("legacy copy: %+v err=%v", p, err)
	}
	// Third run must not duplicate or overwrite.
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("third migrate: %v", err)
	}
	all, _ := s.Prompts().List(ctx)
	if len(all) != 1 {
		t.Fatalf("override rows = %d, want 1", len(all))
	}
}

func TestMaterialsFTSRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if !s.condApplied["materials_fts"] {
		t.Fatalf("materials_fts conditional migration not applied; warnings: %v", s.warnings)
	}

	created, err := s.Materials().Create(ctx, &domain.Material{
		SessionID: "sid1", Category: "diet", Title: "牛肉面一碗",
		Summary: "午餐吃了牛肉面", Body: `{"text":"中午吃了牛肉面大概600大卡"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Materials().Create(ctx, &domain.Material{
		SessionID: "sid1", Category: "note", Title: "别的笔记", Summary: "无关内容", Body: "完全不同的文字",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, ok, err := s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "牛肉面"})
	if err != nil || !ok {
		t.Fatalf("fts search: ok=%v err=%v", ok, err)
	}
	if len(res) != 1 || res[0].ID != created.ID || res[0].Title != "牛肉面一碗" {
		t.Fatalf("fts hit: %+v", res)
	}
	// Category filter narrows; wrong category misses.
	if res, ok, _ = s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "牛肉面", Category: "note"}); !ok || len(res) != 0 {
		t.Fatalf("category filter: ok=%v res=%+v", ok, res)
	}
	// Cross-session isolation.
	if res, ok, _ = s.SearchMaterialsFTS(ctx, "other-sid", domain.SearchQuery{Term: "牛肉面"}); !ok || len(res) != 0 {
		t.Fatalf("session isolation: ok=%v res=%+v", ok, res)
	}
	// Terms under 3 runes can't use trigram — ok=false → caller falls back.
	if _, ok, _ = s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "牛肉"}); ok {
		t.Fatal("2-rune term must report ok=false (substring fallback)")
	}

	// Update flows through the sync trigger.
	created.Title = "换成了咖喱饭"
	created.Summary = "晚餐咖喱饭"
	created.Body = `{"text":"晚上吃了咖喱饭"}`
	if _, err := s.Materials().Update(ctx, "sid1", created.ID, created); err != nil {
		t.Fatal(err)
	}
	if res, ok, _ = s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "咖喱饭"}); !ok || len(res) != 1 {
		t.Fatalf("post-update hit: ok=%v res=%+v", ok, res)
	}
	if res, ok, _ = s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "牛肉面"}); !ok || len(res) != 0 {
		t.Fatalf("post-update stale hit: ok=%v res=%+v", ok, res)
	}

	// Delete flows through the sync trigger.
	if err := s.Materials().Delete(ctx, "sid1", created.ID); err != nil {
		t.Fatal(err)
	}
	if res, ok, _ = s.SearchMaterialsFTS(ctx, "sid1", domain.SearchQuery{Term: "咖喱饭"}); !ok || len(res) != 0 {
		t.Fatalf("post-delete hit: ok=%v res=%+v", ok, res)
	}
}

func TestChatMessageStatusRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	th, err := s.Chats().CreateThread(ctx, &domain.ChatThread{SessionID: "sid1", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Chats().AppendMessages(ctx, []domain.ChatMessage{
		{ID: "u1", ThreadID: th.ID, SessionID: "sid1", Role: domain.RoleUser, Content: "hi"},
		{ID: "a1", ThreadID: th.ID, SessionID: "sid1", Role: domain.RoleAssistant, Status: domain.MsgStatusPending},
	})
	if err != nil {
		t.Fatal(err)
	}

	// GetMessage sees the pending placeholder; wrong sid must miss.
	m, err := s.Chats().GetMessage(ctx, "sid1", "a1")
	if err != nil || m.Status != domain.MsgStatusPending {
		t.Fatalf("get placeholder: %+v err=%v", m, err)
	}
	if _, err := s.Chats().GetMessage(ctx, "other-sid", "a1"); err != domain.ErrNotFound {
		t.Fatalf("cross-session get should be ErrNotFound, got %v", err)
	}

	// Finalize: content + toolEvents + status in one update.
	content, events, done := "answer", `[{"type":"tool_start"}]`, domain.MsgStatusDone
	err = s.Chats().UpdateMessage(ctx, "sid1", "a1", domain.ChatMessageUpdate{
		Content: &content, ToolEvents: &events, Status: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ = s.Chats().GetMessage(ctx, "sid1", "a1")
	if m.Content != "answer" || m.ToolEvents != events || m.Status != domain.MsgStatusDone {
		t.Fatalf("after update: %+v", m)
	}
	// User message keeps "" (legacy-compatible ≡ done) and lists fine.
	msgs, _ := s.Chats().ListMessages(ctx, th.ID, "sid1", "", 10)
	if len(msgs) != 2 || msgs[1].Status != "" {
		t.Fatalf("list: %+v", msgs)
	}

	// Startup sweep: a fresh pending row flips to error; done rows untouched.
	_ = s.Chats().AppendMessages(ctx, []domain.ChatMessage{
		{ID: "a2", ThreadID: th.ID, SessionID: "sid1", Role: domain.RoleAssistant, Status: domain.MsgStatusPending},
	})
	n, err := s.Chats().FailPendingMessages(ctx)
	if err != nil || n != 1 {
		t.Fatalf("sweep: n=%d err=%v", n, err)
	}
	m, _ = s.Chats().GetMessage(ctx, "sid1", "a2")
	if m.Status != domain.MsgStatusError {
		t.Fatalf("swept status: %q", m.Status)
	}
	m, _ = s.Chats().GetMessage(ctx, "sid1", "a1")
	if m.Status != domain.MsgStatusDone {
		t.Fatalf("done row must survive sweep: %q", m.Status)
	}
}

// TestBlockLockAndNoteRoundTrip pins the batch-A fields onto the wire. Blocks
// live inside a JSON blob, so a field that fails to round-trip fails silently —
// no column, no error, just a value that quietly becomes the zero value.
func TestBlockLockAndNoteRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	dur := 95
	tm := "10:10"
	in := []domain.TimeBlock{
		{
			ID: "locked", Title: "CS 201 数据结构（课）", Type: domain.BlockAppointment,
			Time: &tm, DurationMin: &dur, TimeMode: domain.TimeFloating,
			LockLevel: domain.LockHard, LockReason: "课程时间由课表决定",
			LockSource: domain.LockSourceDerived,
		},
		{
			// Unlocked by hand: LockNone must survive, otherwise a re-read would
			// helpfully lock the user's class right back up.
			ID: "unlocked", Title: "社团例会", Type: domain.BlockAppointment,
			TimeMode:  domain.TimeFloating,
			LockLevel: domain.LockNone, LockSource: domain.LockSourceUser,
		},
		{
			ID: "refished", Title: "锻炼", Type: domain.BlockTask,
			TimeMode: domain.TimeFloating, Note: "只做了一半，手腕疼",
			RescheduledFrom: "orig-1", RescheduleCount: 2,
		},
	}
	if _, err := s.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: "sid-lock", Date: "2026-07-26", Blocks: in,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.DayPlans().Get(ctx, "sid-lock", "2026-07-26")
	if err != nil || len(got.Blocks) != 3 {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	by := map[string]domain.TimeBlock{}
	for _, b := range got.Blocks {
		by[b.ID] = b
	}

	if b := by["locked"]; b.LockLevel != domain.LockHard ||
		b.LockReason != "课程时间由课表决定" || b.LockSource != domain.LockSourceDerived {
		t.Errorf("hard lock lost: level=%q reason=%q source=%q", b.LockLevel, b.LockReason, b.LockSource)
	}
	if b := by["unlocked"]; b.LockLevel != domain.LockNone || b.LockSource != domain.LockSourceUser {
		t.Errorf("deliberate unlock lost: level=%q source=%q", b.LockLevel, b.LockSource)
	}
	if b := by["refished"]; b.Note != "只做了一半，手腕疼" ||
		b.RescheduledFrom != "orig-1" || b.RescheduleCount != 2 {
		t.Errorf("note/refish lost: note=%q from=%q count=%d", b.Note, b.RescheduledFrom, b.RescheduleCount)
	}

	// A legacy block carries no lock at all; LockUnset is what tells DeriveLock
	// it still has work to do, so it must not be confused with LockNone.
	if _, err := s.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: "sid-legacy", Date: "2026-07-26",
		Blocks: []domain.TimeBlock{{ID: "old", Title: "旧块", Type: domain.BlockTask}},
	}); err != nil {
		t.Fatal(err)
	}
	legacy, _ := s.DayPlans().Get(ctx, "sid-legacy", "2026-07-26")
	if legacy.Blocks[0].LockLevel != domain.LockUnset {
		t.Errorf("legacy block should read back as LockUnset, got %q", legacy.Blocks[0].LockLevel)
	}
}

// TestOpLogDomain covers both halves: an explicit domain survives, and a row
// written without one still reads back classified (existing rows predate the
// column and must not skew rapport scoring).
func TestOpLogDomain(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	explicit := &domain.OperationLog{
		SessionID: "sid-d", Actor: domain.ActorAgent, Action: "plan_add",
		Domain: domain.OpDomainCare, Summary: "explicit wins",
	}
	if err := s.OpLogs().Add(ctx, explicit); err != nil {
		t.Fatal(err)
	}
	got, err := s.OpLogs().Get(ctx, "sid-d", explicit.ID)
	if err != nil || got.Domain != domain.OpDomainCare {
		t.Fatalf("explicit domain: %q err=%v", got.Domain, err)
	}

	derived := &domain.OperationLog{
		SessionID: "sid-d", Actor: domain.ActorUser, Action: "memory_add", Summary: "derive me",
	}
	if err := s.OpLogs().Add(ctx, derived); err != nil {
		t.Fatal(err)
	}
	if derived.Domain != domain.OpDomainArchive {
		t.Errorf("Add should stamp the derived domain onto the struct, got %q", derived.Domain)
	}
	got, _ = s.OpLogs().Get(ctx, "sid-d", derived.ID)
	if got.Domain != domain.OpDomainArchive {
		t.Errorf("derived domain: %q", got.Domain)
	}

	// Simulate a pre-migration row: column present but empty.
	if _, err := s.exec(ctx,
		`INSERT INTO operation_logs (id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at)
		 VALUES (?, ?, ?, ?, '', '', '', ?, '', ?, '', ?)`,
		"legacy-1", "sid-d", domain.ActorUser, "rule_create", "legacy row", domain.OpStatusOK, nowMillis()); err != nil {
		t.Fatal(err)
	}
	got, err = s.OpLogs().Get(ctx, "sid-d", "legacy-1")
	if err != nil || got.Domain != domain.OpDomainHabit {
		t.Fatalf("legacy row should derive habit, got %q err=%v", got.Domain, err)
	}
	logs, _ := s.OpLogs().List(ctx, "sid-d", 10)
	for _, l := range logs {
		if l.Domain == "" {
			t.Errorf("List returned an unclassified row: action=%q", l.Action)
		}
	}
}
