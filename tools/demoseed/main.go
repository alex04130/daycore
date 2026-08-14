// Command demoseed generates a seeded demo.db for frontend acceptance.
// It is a dev tool, not part of the API: run it to (re)generate the throwaway
// demo database that run-frontends.sh loads via DB_DSN.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage"

	_ "daycore/internal/storage/sqlstore" // registers "sqlite"
	_ "modernc.org/sqlite"                // raw backdating driver "sqlite"
	_ "time/tzdata"
)

// demoSessionID is FIXED so the signed token (and the hub's dc_sid cookie) stay
// stable across regenerations: the sid + COOKIE_SECRET together determine the
// token, and COOKIE_SECRET comes from .env (stable).
const demoSessionID = "QYskIbkUypmAqmgJbqhE2zOIp5edTJSOoe-gXanLHP8"

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "file:/home/alex/daycore/demo.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	store, err := storage.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	moodIDs, opIDs, err := seed(ctx, store)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	if err := backdate(dsn, demoSessionID, moodIDs, opIDs); err != nil {
		log.Fatalf("backdate: %v", err)
	}
	fmt.Printf("demo.db seeded: %d moods + %d ops backdated over 15 days\n", len(moodIDs), len(opIDs))
}

type blockSeed struct {
	id, t, title string
	typ          domain.BlockType
	dur          int
	origin       string
	completed    bool
	lock         domain.LockLevel
	lockSource   string
}

func seed(ctx context.Context, store domain.Store) (moodIDs, opIDs []string, err error) {
	sid := demoSessionID
	if _, err := store.Sessions().GetOrCreate(ctx, sid); err != nil {
		return nil, nil, err
	}

	// ── session: assistant 小禾 + timezone Asia/Shanghai + family themes ──
	prefs, _ := json.Marshal(map[string]any{
		"timezone":       "Asia/Shanghai",
		"timezoneSource": "user",
		"themeByFamily": map[string]string{
			"ting":          "night",
			"zhiyu":         "sunset",
			"liuli":         "sky",
			"liuli-classic": "sky",
		},
	})
	prefsStr := string(prefs)
	if _, err := store.Sessions().Update(ctx, sid, domain.SessionUpdate{
		AssistantName: strp("小禾"),
		// The demo content is all zh-CN. Pin the session's current language so
		// the catalog-driven surfaces (mood names, lock reasons) render Chinese,
		// and so the sign-in merge carries it into the canonical session — a
		// minted session otherwise defaults to "" and falls back to the
		// deployment locale, making the demo read half-English after login.
		Language:    strp("zh-CN"),
		Preferences: &prefsStr,
	}); err != nil {
		return nil, nil, err
	}

	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(shanghai)
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	dayAfter := now.AddDate(0, 0, 2).Format("2006-01-02")

	// ── courses ──
	type courseSeed struct{ id, name, code string }
	courseSeeds := []courseSeed{
		{"demo-cs101", "数据结构", "CS 101"},
		{"demo-math", "高等数学", "MATH 201"},
		{"demo-eng", "大学英语", "ENGL 102"},
		{"demo-phys", "大学物理", "PHYS 110"},
		{"demo-hist", "中国近现代史", "HIST 100"},
	}
	courseIDByCanvas := map[string]string{}
	for _, c := range courseSeeds {
		saved, err := store.Courses().UpsertByCanvasID(ctx, &domain.Course{
			SessionID: sid, CanvasID: c.id, Name: c.name, CourseCode: c.code,
		})
		if err != nil {
			return nil, nil, err
		}
		courseIDByCanvas[c.id] = saved.ID
	}

	// ── assignments (natural clock times) ──
	loc := shanghai
	tmr2359 := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 0, 0, loc).AddDate(0, 0, 1)
	mid10 := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc).AddDate(0, 0, 5)
	far2359 := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 0, 0, loc).AddDate(0, 0, 12)
	type asgnSeed struct {
		cid, title string
		due        time.Time
		pts        float64
	}
	asgnSeeds := []asgnSeed{
		{"demo-cs101", "二叉树遍历作业", tmr2359, 100},
		{"demo-math", "定积分习题集", mid10, 50},
		{"demo-eng", "论文初稿", far2359, 100},
		{"demo-phys", "力学实验报告", mid10, 40},
		{"demo-cs101", "期末项目：最小堆实现", far2359, 150},
		{"demo-hist", "读书笔记", tmr2359, 30},
	}
	for i, a := range asgnSeeds {
		due, pts := a.due, a.pts
		if _, err := store.Assignments().UpsertByCanvasID(ctx, &domain.Assignment{
			SessionID: sid, CanvasID: fmt.Sprintf("demo-asgn-%d", i), Title: a.title,
			DueAt: &due, PointsPossible: &pts, CourseID: courseIDByCanvas[a.cid],
			Source: "canvas", Status: domain.AssignmentPending,
		}); err != nil {
			return nil, nil, err
		}
	}

	// ── recurring rules ──
	rules := []domain.ScheduleRule{
		{SessionID: sid, Title: "早读英语", Type: domain.BlockTask, Time: strp("07:00"), DurationMin: intp(30), TimeMode: domain.TimeMode("floating"), Kind: "recurring", Freq: "weekly", ByWeekday: []int{1, 2, 3, 4, 5}, StartDate: today, Active: true, Source: "user"},
		{SessionID: sid, Title: "每周三 高等数学课", Type: domain.BlockAppointment, Time: strp("09:00"), DurationMin: intp(90), TimeMode: domain.TimeMode("floating"), Kind: "recurring", Freq: "weekly", ByWeekday: []int{3}, StartDate: today, Active: true, Source: "user"},
		{SessionID: sid, Title: "每周一三五 晨跑", Type: domain.BlockTask, Time: strp("07:00"), DurationMin: intp(30), TimeMode: domain.TimeMode("floating"), Kind: "recurring", Freq: "weekly", ByWeekday: []int{1, 3, 5}, StartDate: today, Active: true, Source: "user"},
	}
	for i := range rules {
		if _, err := store.Rules().Create(ctx, &rules[i]); err != nil {
			return nil, nil, err
		}
	}

	// ── plan blocks (today / tomorrow / day-after) ──
	days := []struct {
		date   string
		blocks []blockSeed
	}{
		{today, []blockSeed{
			{"demo-b1", "07:00", "晨跑", domain.BlockTask, 30, domain.OriginManual, true, domain.LockNone, domain.LockSourceDerived},
			{"demo-b2", "09:00", "高等数学课", domain.BlockAppointment, 90, domain.OriginManual, false, domain.LockHard, domain.LockSourceDerived},
			{"demo-b3", "12:00", "午饭", domain.BlockMeal, 60, domain.OriginManual, false, domain.LockNone, ""},
			{"demo-b4", "15:00", "和导师开会", domain.BlockAppointment, 60, domain.OriginManual, false, domain.LockSoft, domain.LockSourceUser},
			{"demo-b5", "16:30", "写作业", domain.BlockTask, 60, domain.OriginAuto, false, domain.LockNone, ""},
			{"demo-b6", "20:00", "读论文", domain.BlockTask, 45, domain.OriginManual, false, domain.LockNone, ""},
		}},
		{tomorrow, []blockSeed{
			{"demo-b7", "10:00", "交作业前检查", domain.BlockTask, 30, domain.OriginManual, false, domain.LockNone, ""},
			{"demo-b8", "14:00", "英语口语练习", domain.BlockTask, 45, domain.OriginAuto, false, domain.LockNone, ""},
			{"demo-b9", "19:00", "社团例会", domain.BlockAppointment, 60, domain.OriginManual, false, domain.LockHard, domain.LockSourceUser},
		}},
		{dayAfter, []blockSeed{
			{"demo-b10", "09:00", "数据结构课", domain.BlockAppointment, 90, domain.OriginManual, false, domain.LockHard, domain.LockSourceDerived},
			{"demo-b11", "15:00", "复习笔记", domain.BlockTask, 60, domain.OriginManual, false, domain.LockNone, ""},
		}},
	}
	for _, d := range days {
		blocks := make([]domain.TimeBlock, 0, len(d.blocks))
		for _, b := range d.blocks {
			blocks = append(blocks, domain.TimeBlock{
				ID: b.id, Date: d.date, Time: &b.t, Title: b.title, Type: b.typ,
				DurationMin: &b.dur, TimeMode: domain.TimeMode("floating"),
				Origin: b.origin, Completed: b.completed,
				LockLevel: b.lock, LockSource: b.lockSource,
			})
		}
		if _, err := store.DayPlans().Upsert(ctx, &domain.DayPlan{
			SessionID: sid, Date: d.date, Blocks: blocks, SourceType: "rules",
		}); err != nil {
			return nil, nil, err
		}
	}

	// ── wishes ──
	wishes := []domain.Wish{
		{SessionID: sid, Title: "学吉他", Note: "先学会基础和弦", EffortMin: 90, Status: domain.WishActive},
		{SessionID: sid, Title: "去海边看一次日出", EffortMin: 240, Status: domain.WishActive},
		{SessionID: sid, Title: "把数据结构考到 A", EffortMin: 120, Status: domain.WishActive},
		{SessionID: sid, Title: "读完《百年孤独》", EffortMin: 60, Status: domain.WishActive},
	}
	for i := range wishes {
		if _, err := store.Wishes().Create(ctx, &wishes[i]); err != nil {
			return nil, nil, err
		}
	}

	// ── materials (diverse categories) ──
	materials := []domain.Material{
		{SessionID: sid, Category: "academic", Title: "数据结构考点整理", Summary: "二叉树、堆、图", Body: "……", Source: "manual", Tags: []string{"考试"}},
		{SessionID: sid, Category: "health", Title: "体检报告", Summary: "血压 120/80", Body: "……", Source: "manual"},
		{SessionID: sid, Category: "media", Title: "想看的电影清单", Summary: "朋友推荐的几部", Body: "……", Source: "manual"},
		{SessionID: sid, Category: "idea", Title: "做一个自动记账工具", Body: "……", Source: "manual"},
		{SessionID: sid, Category: "note", Title: "英语作文模板", Summary: "议论文三段式", Body: "……", Source: "manual"},
	}
	for i := range materials {
		if _, err := store.Materials().Create(ctx, &materials[i]); err != nil {
			return nil, nil, err
		}
	}

	// ── memories (preference + open_loop kinds) ──
	memories := []domain.MemoryFact{
		{SessionID: sid, Fact: "喜欢在安静的环境里学习", Source: "user", Type: "preference"},
		{SessionID: sid, Fact: "周四下午有固定社团活动", Source: "user", Type: ""},
		{SessionID: sid, Fact: "想买一双轻便的跑鞋", Source: "user", Type: "open_loop"},
	}
	for i := range memories {
		if _, err := store.Memory().AddFact(ctx, &memories[i]); err != nil {
			return nil, nil, err
		}
	}

	// ── one pending proposal ──
	deliveredAt := now
	expiresAt := now.Add(6 * time.Hour)
	if err := store.Proposals().Create(ctx, &domain.Proposal{
		ID: "demo-proposal-1", State: domain.ProposalPending, Level: domain.LevelL2, Kind: domain.KindCard,
		Title: "给「读论文」留个专注块？", Summary: "今晚 20:00 的读论文还没定读哪篇",
		Reason: "你之前说想精读那篇综述", Evidence: "连续三天没动", BType: domain.BlockTask,
		SessionID: sid, MergeKey: "demo-read-paper", DeliveredAt: &deliveredAt,
		ExpiresAt: expiresAt, TTLPolicy: domain.TTLSilenceRejects, Origin: domain.OriginDaemon,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return nil, nil, err
	}

	// ── 15 moods + 15 ops (created now, backdated afterwards) ──
	moodKinds := []string{"calm", "tired", "happy", "excited", "neutral", "stressed", "anxious", "down", "loved", "calm", "tired", "happy", "excited", "neutral", "stressed"}
	for _, m := range moodKinds {
		created, err := store.Moods().Create(ctx, &domain.MoodCheckin{
			SessionID: sid, Mood: m, Source: domain.MoodSourceUser,
		})
		if err != nil {
			return nil, nil, err
		}
		moodIDs = append(moodIDs, created.ID)
	}

	opActions := []string{"plan_upsert", "mood_record", "wish_add", "material_add", "rule_create", "plan_add", "plan_update", "memory_add", "plan_upsert", "mood_record", "wish_add", "material_add", "plan_add", "rule_create", "memory_add"}
	opDomains := []string{domain.OpDomainSchedule, domain.OpDomainCare, domain.OpDomainArchive, domain.OpDomainArchive, domain.OpDomainSchedule, domain.OpDomainSchedule, domain.OpDomainSchedule, domain.OpDomainArchive, domain.OpDomainSchedule, domain.OpDomainCare, domain.OpDomainArchive, domain.OpDomainArchive, domain.OpDomainSchedule, domain.OpDomainSchedule, domain.OpDomainArchive}
	opSummaries := []string{"排了今天的日程", "打卡：平静", "记下想做的事", "存了一条资料", "加了每周规则", "加了一个块", "改了一个块", "记下了一件事", "排了今天的日程", "打卡：疲惫", "记下想做的事", "存了一条资料", "加了一个块", "加了每周规则", "记下了一件事"}
	for i, action := range opActions {
		op := &domain.OperationLog{
			ID: fmt.Sprintf("demo-op-%d", i), SessionID: sid, Actor: domain.ActorUser, Action: action,
			Domain: opDomains[i], Date: today, Summary: opSummaries[i], Status: "ok", CreatedAt: now,
		}
		if err := store.OpLogs().Add(ctx, op); err != nil {
			return nil, nil, err
		}
		opIDs = append(opIDs, op.ID)
	}

	return moodIDs, opIDs, nil
}

// backdate spreads the created moods + ops over the last 15 days via raw SQL,
// because the store Create methods stamp created_at = now and offer no backdate.
func backdate(dsn, sid string, moodIDs, opIDs []string) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now()
	for i, id := range moodIDs {
		ts := now.AddDate(0, 0, -i).UnixMilli()
		if _, err := db.Exec(`UPDATE mood_checkins SET created_at = ? WHERE id = ? AND session_id = ?`, ts, id, sid); err != nil {
			return err
		}
	}
	for i, id := range opIDs {
		ts := now.AddDate(0, 0, -i).UnixMilli()
		if _, err := db.Exec(`UPDATE operation_logs SET created_at = ? WHERE id = ? AND session_id = ?`, ts, id, sid); err != nil {
			return err
		}
	}
	return nil
}
