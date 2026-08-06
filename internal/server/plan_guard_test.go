package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/auth"
	"daycore/internal/domain"
)

// Every plan write passes one gate. Before it existed, domain.Movable,
// TimeBlock.Frozen, PhaseIn and PetrifyLine were all written, all tested, and
// all had zero production callers — so a user could drag a hard-locked class
// and rewrite a block from three days ago, and nothing anywhere said no.
//
// The table covers both directions on purpose. A gate that refuses everything
// passes every "is it refused?" test and breaks the product, so the allowed
// rows carry as much weight as the refused ones.

func seedBlock(t *testing.T, s *Server, sid, date string, b domain.TimeBlock) {
	t.Helper()
	if _, err := s.store.DayPlans().Upsert(context.Background(), &domain.DayPlan{
		SessionID: sid, Date: date, SourceType: "manual", Blocks: []domain.TimeBlock{b},
	}); err != nil {
		t.Fatal(err)
	}
}

func hhmm(v string) *string { return &v }
func mins(v int) *int       { return &v }

func TestPlanGate(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	loc := s.planLocation()
	today := time.Now().In(loc).Format("2006-01-02")
	longAgo := time.Now().In(loc).AddDate(0, 0, -3).Format("2006-01-02")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	cases := []struct {
		name    string
		date    string
		block   domain.TimeBlock
		action  planAction
		actor   string
		wantErr string // "" = must be allowed
	}{
		{
			name: "用户挪不动硬锁的课", date: today, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"time": "15:00"}},
			wantErr: "locked",
		},
		{
			name: "软锁不确认则拒", date: today, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "和导师见面", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockSoft},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"time": "15:00"}},
			wantErr: "locked",
		},
		{
			name: "软锁确认后放行", date: today, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "和导师见面", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockSoft},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"time": "15:00"}, Confirm: true},
		},
		{
			// 硬锁不是「助手不许碰」，是「时间由课表决定」—— agent 重排要能动它。
			name: "agent 挪得动硬锁块", date: today, actor: domain.ActorAgent,
			block:  domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"time": "15:00"}},
		},
		{
			// 锁守的是**时间**，不是存在。默认理由文案自己写着「课程时间由课表
			// 决定」—— 请假是「这次我不去」，不是「我要改课表」。而请假走的正是
			// remove（规则展开的块转成墓碑，规则本身还在），所以锁门必须放行它，
			// 否则硬锁就成了一条没有出口的死路。
			name: "硬锁的课可以请假（remove 转墓碑）", date: today, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard, RuleID: "r1"},
			action: planAction{Action: "remove", Match: map[string]any{"id": "b"}},
		},
		{
			// 同理，改标题不是改时间。
			name: "硬锁的课可以改标题", date: today, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "高等数学"}},
		},
		{
			name: "硬锁的课可以打勾", date: today, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"completed": true}},
		},
		{
			// 但时长是时间的一部分。
			name: "硬锁的课改不了时长", date: today, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60), LockLevel: domain.LockHard},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"duration_min": 30}},
			wantErr: "locked",
		},
		{
			// 石化的 remove 仍然拦：已经发生的事没法「不发生」。
			name: "用户改不了三天前的块", date: longAgo, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "改过了"}},
			wantErr: "petrified",
		},
		{
			name: "用户删不掉三天前的块", date: longAgo, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action:  planAction{Action: "remove", Match: map[string]any{"id": "b"}},
			wantErr: "petrified",
		},
		{
			// 对账是翻篇的最后一笔，不是编辑。拦住它等于让昨天永远半完成。
			name: "石化块仍可对账（改 completed）", date: longAgo, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"completed": true}},
		},
		{
			// 混着改就不行 —— 否则「顺手改个标题」可以搭对账的便车。
			name: "对账不能夹带别的改动", date: longAgo, actor: domain.ActorUser,
			block:   domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"completed": true, "title": "顺便改了"}},
			wantErr: "petrified",
		},
		{
			// 撤销走 ActorSystem，它必须能重建冻结的块，否则撤销在最需要的时候失灵。
			name: "system 改得动石化块", date: longAgo, actor: domain.ActorSystem,
			block:  domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "撤销恢复"}},
		},
		{
			name: "agent 也改不了石化块", date: longAgo, actor: domain.ActorAgent,
			block:   domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action:  planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "改过了"}},
			wantErr: "petrified",
		},
		{
			name: "明天的块随便改", date: tomorrow, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "改过了"}},
		},
		{
			// 新增不触碰任何已存在的东西，哪怕落在已石化的那一天。
			name: "石化日仍可新增", date: longAgo, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "原标题", Time: hhmm("09:00"), DurationMin: mins(60)},
			action: planAction{Action: "add", Block: map[string]any{"id": "n1", "title": "补记"}},
		},
		{
			// 没有时间的块无法定相位；拿不准就放行，把不确定算在用户那边。
			name: "无时间的块不受石化门管", date: longAgo, actor: domain.ActorUser,
			block:  domain.TimeBlock{ID: "b", Title: "不定时"},
			action: planAction{Action: "update", Match: map[string]any{"id": "b"}, Changes: map[string]any{"title": "改过了"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// No per-case date juggling: seedBlock upserts the whole day, so
			// each case wipes the last. An earlier draft shifted every case onto
			// its own date and pushed the "today" rows three months into the
			// past, where the petrify gate answered before the lock gate ever
			// ran — the test was measuring the wrong rule and passing anyway on
			// the cases that happened to want a refusal.
			date := c.date
			seedBlock(t, s, sid, date, c.block)
			_, _, _, err := s.applyPlanPatch(ctx, sid, date, "zh-CN", c.action, c.actor)

			var blocked *planBlocked
			switch {
			case c.wantErr == "":
				if err != nil {
					t.Fatalf("should have been allowed, got %v", err)
				}
			case !errors.As(err, &blocked):
				t.Fatalf("want %q refusal, got %v", c.wantErr, err)
			case blocked.Code != c.wantErr:
				t.Fatalf("refused with %q, want %q", blocked.Code, c.wantErr)
			}

			// A refusal must leave the plan untouched — a gate that rejects
			// after mutating is worse than no gate.
			if c.wantErr != "" {
				p, err := s.store.DayPlans().Get(ctx, sid, date)
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Blocks) != 1 || p.Blocks[0].Title != c.block.Title {
					t.Errorf("a refused write still changed the plan: %+v", p.Blocks)
				}
			}
		})
	}
}

// The HTTP surface of the same refusal. 409 rather than 500 (it is not an
// outage) and rather than 403 (nothing is wrong with who is asking); the body
// says which rule refused and whether there is a way through.
func TestPlanGateHTTPEnvelope(t *testing.T) {
	s, sid := newAgentTestServer(t)
	// newAgentTestServer leaves Cookies nil — it drives the agent loop directly
	// and never signs anything. An HTTP test needs a real signer to mint the
	// session header, so attach one here rather than widening that helper for
	// every caller that does not care.
	s.cookies = auth.NewCookieSigner("plan-gate-test-secret")

	loc := s.planLocation()
	today := time.Now().In(loc).Format("2006-01-02")
	seedBlock(t, s, sid, today, domain.TimeBlock{
		ID: "b", Title: "高数（课）", Time: hhmm("09:00"), DurationMin: mins(60),
		LockLevel: domain.LockHard, LockReason: "课表决定",
	})

	body := `{"date":"` + today + `","action":{"action":"update","match":{"id":"b"},"changes":{"time":"15:00"}}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/plan", strings.NewReader(body))
	req.Header.Set("X-Session-Token", s.cookies.Sign(sid))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409. body: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{
		"error": "blocked", "code": "locked", "blockId": "b",
		"lockLevel": "hard", "lockReason": "课表决定", "confirmable": false,
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Error("no message — the client has nothing to show the user")
	}
}

// The gate guards lockLevel, and until this batch nothing ever set one:
// domain.DeriveLock had no production caller, so every block in the database
// carried an empty level and a class imported from a timetable was as movable
// as a note to self. Derivation now runs on the one path every write already
// takes (normalizePlanBlocks).
func TestLockIsDerivedOnWrite(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	date := time.Now().In(s.planLocation()).AddDate(0, 0, 1).Format("2006-01-02")

	// An appointment whose title reads as a class: hard by the shared rules.
	if _, _, _, err := s.applyPlanPatch(ctx, sid, date, "zh-CN", planAction{
		Action: "add",
		Block: map[string]any{
			"id": "c1", "title": "高等数学（课）", "type": "appointment",
			"time": "09:00", "duration_min": 60,
		},
	}, domain.ActorUser); err != nil {
		t.Fatal(err)
	}
	p, err := s.store.DayPlans().Get(ctx, sid, date)
	if err != nil || len(p.Blocks) != 1 {
		t.Fatalf("plan: %v blocks=%d", err, len(p.Blocks))
	}
	got := p.Blocks[0]
	if got.LockLevel != domain.LockHard {
		t.Errorf("lockLevel = %q, want hard — derivation did not run on the write path", got.LockLevel)
	}
	if got.LockSource != domain.LockSourceDerived {
		t.Errorf("lockSource = %q, want derived", got.LockSource)
	}

	// And the gate now has something to act on: retiming it is refused.
	_, _, _, err = s.applyPlanPatch(ctx, sid, date, "zh-CN", planAction{
		Action: "update", Match: map[string]any{"id": "c1"},
		Changes: map[string]any{"time": "15:00"},
	}, domain.ActorUser)
	var blocked *planBlocked
	if !errors.As(err, &blocked) || blocked.Code != "locked" {
		t.Fatalf("retiming a derived-hard class: got %v, want a locked refusal", err)
	}

	// A plain task is not a class and must stay free.
	if _, _, _, err := s.applyPlanPatch(ctx, sid, date, "zh-CN", planAction{
		Action: "add",
		Block:  map[string]any{"id": "t1", "title": "写作业", "type": "task", "time": "14:00", "duration_min": 60},
	}, domain.ActorUser); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.applyPlanPatch(ctx, sid, date, "zh-CN", planAction{
		Action: "update", Match: map[string]any{"id": "t1"},
		Changes: map[string]any{"time": "16:00"},
	}, domain.ActorUser); err != nil {
		t.Errorf("a task should be freely movable: %v", err)
	}
}

// A derived reason is a rendering of a level, so it follows the reader's
// language. A user-written one is somebody's own words and must not be
// translated on their behalf.
func TestDerivedLockReasonFollowsTheReader(t *testing.T) {
	blocks := []domain.TimeBlock{
		{ID: "a", LockLevel: domain.LockHard, LockSource: domain.LockSourceDerived, LockReason: "课程时间由课表决定"},
		{ID: "b", LockLevel: domain.LockHard, LockSource: domain.LockSourceUser, LockReason: "别动，答应了室友"},
	}
	localizeLockReasons(blocks, "en-US")
	if blocks[0].LockReason != domain.DefaultLockReason(domain.LockHard, "en-US") {
		t.Errorf("derived reason stayed %q — a language switch would show the wrong one", blocks[0].LockReason)
	}
	if blocks[1].LockReason != "别动，答应了室友" {
		t.Errorf("a user's own words were translated: %q", blocks[1].LockReason)
	}
}
