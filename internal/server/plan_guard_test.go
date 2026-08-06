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
			_, _, _, err := s.applyPlanPatch(ctx, sid, date, c.action, c.actor)

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
