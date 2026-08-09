package server

import (
	"context"
	"encoding/json"
	"fmt"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// toolResult is one companion-tool execution outcome.
type toolResult struct {
	OK      bool
	Data    map[string]any
	Summary string
	ErrMsg  string
	OpID    string
	// ForModel, when set, replaces JSON() as the tool-message content the model
	// sees. It exists for results that carry third-party text: the client gets
	// the clean Data while the model gets the same bytes wrapped by
	// untrustedWrap.
	ForModel string
}

// forModelJSON is the content injected back into the conversation.
func (t toolResult) forModelJSON() string {
	if t.ForModel != "" {
		return t.ForModel
	}
	return t.JSON()
}

func (t toolResult) JSON() string {
	m := map[string]any{"ok": t.OK}
	for k, v := range t.Data {
		m[k] = v
	}
	if t.ErrMsg != "" {
		m["error"] = t.ErrMsg
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func toolFail(format string, args ...any) toolResult {
	return toolResult{OK: false, ErrMsg: fmt.Sprintf(format, args...)}
}

func schemaObj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func schemaStr(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func schemaInt(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// schemaEnum is a string parameter restricted to a fixed set.
//
// The caller passes an ALREADY SORTED slice and this does not sort it again —
// deliberately, so there is one place that owns the ordering. Sorting in two
// places is how two orderings appear; and the ordering is load-bearing, because
// these bytes go into the tool band, which renders before the system prompt and
// takes the whole cached prefix with it when it changes.
func schemaEnum(desc string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

// visiblePlan returns the full day plan with only visible (non-hidden) blocks.
func visiblePlan(dp *domain.DayPlan) *domain.DayPlan {
	if dp == nil {
		return nil
	}
	visible := make([]domain.TimeBlock, 0, len(dp.Blocks))
	for _, b := range dp.Blocks {
		if !b.Hidden {
			visible = append(visible, b)
		}
	}
	return &domain.DayPlan{
		ID: dp.ID, SessionID: dp.SessionID, Date: dp.Date,
		Blocks: visible, SourceType: dp.SourceType, Note: dp.Note,
		CreatedAt: dp.CreatedAt, UpdatedAt: dp.UpdatedAt,
	}
}

// runCompanionTool dispatches a tool call to the correct handler.
func (s *Server) runCompanionTool(ctx context.Context, sid, locale, tz string, call ai.ToolCall) toolResult {
	switch call.Name {
	case "plan_add":
		return s.toolPlanAdd(ctx, sid, locale, tz, call.Arguments)
	case "plan_update":
		return s.toolPlanPatch(ctx, sid, locale, call.Arguments, "update")
	case "plan_remove":
		return s.toolPlanPatch(ctx, sid, locale, call.Arguments, "remove")
	case "rule_upsert":
		return s.toolRuleUpsert(ctx, sid, tz, call.Arguments)
	case "rule_remove":
		return s.toolRuleRemove(ctx, sid, call.Arguments)
	case "memory_add":
		return s.toolMemoryAdd(ctx, sid, call.Arguments)
	case "memory_remove":
		return s.toolMemoryRemove(ctx, sid, call.Arguments)
	case "assignment_upsert":
		return s.toolAssignmentUpsert(ctx, sid, call.Arguments)
	case "wish_add":
		return s.toolWishAdd(ctx, sid, call.Arguments)
	case "set_home_location":
		return s.toolSetHomeLocation(ctx, sid, call.Arguments)
	case "mood_record":
		return s.toolMoodRecord(ctx, sid, tz, call.Arguments)
	case "material_add":
		return s.toolMaterialAdd(ctx, sid, call.Arguments)
	case "get_weather":
		return s.toolGetWeather(ctx, sid, locale, call.Arguments)
	case "web_search":
		return s.toolWebSearch(ctx, sid, call.Arguments)
	case "list_upcoming":
		return s.toolListUpcoming(ctx, sid, call.Arguments)
	default:
		return toolFail("unknown tool: %s", call.Name)
	}
}

// companionToolDefs is the agent's tool belt. interactive=false leaves out
// propose_decision — a sink whose client cannot answer a card (channel replies)
// must not offer the tool at all, or the agent would wait out the full decision
// timeout for an answer that can never arrive.
// companionToolDefs builds the tool band for one conversation round.
//
// weatherIDs / searchIDs are the sources currently usable, already sorted. Two
// rules come out of docs/specs/transport.md and both are about the prompt cache
// rather than tidiness — the tool band renders BEFORE the system prompt, and
// the ephemeral breakpoint sits on the system block, so a band whose bytes
// differ between rounds rewrites that breakpoint and all four layers behind it:
//
//   - the enum is the sorted id list, never a map iteration and never anything
//     carrying a timestamp;
//   - a capability with NO usable source is left out of the band entirely
//     rather than offered and failing. The precedent is the interactive gate
//     below: a tool that cannot succeed costs a round trip and leaves its error
//     in the conversation.
func companionToolDefs(caps ai.Capabilities, interactive bool, weatherIDs, searchIDs []string) []ai.ToolDef {
	blockType := map[string]any{"type": "string", "enum": []string{"task", "appointment", "break", "relax", "meal"}, "description": "块类型，默认 task"}
	// The mood enum is derived from the domain registry, not copied: the
	// twelve ids are a product decision with one owner (mood_kind.go), and a
	// second list here is how the prompt vocabulary drifted from the check-in
	// vocabulary once already.
	moodIDs := make([]string, 0, len(domain.MoodKinds()))
	for _, k := range domain.MoodKinds() {
		moodIDs = append(moodIDs, k.ID)
	}
	tools := []ai.ToolDef{
		{
			Name:        "plan_add",
			Description: "在指定日期的计划里新增一个时间块。只影响这一天；长期/重复安排用 rule_upsert。",
			Parameters: schemaObj(map[string]any{
				"date":         schemaStr("目标日期 YYYY-MM-DD，必须从相对日期对照表取值"),
				"title":        schemaStr("标题"),
				"time":         schemaStr("开始时间 HH:MM；不确定可省略（不定时块）"),
				"type":         blockType,
				"duration_min": schemaInt("时长（分钟），可省略"),
			}, "date", "title"),
		},
		{
			Name:        "plan_update",
			Description: "修改指定日期计划里匹配到的时间块（改时间/标题/时长等）。",
			Parameters: schemaObj(map[string]any{
				"date":    schemaStr("目标日期 YYYY-MM-DD，必须从相对日期对照表取值"),
				"match":   schemaObj(map[string]any{"id": schemaStr("块 id（上下文计划里有，最可靠）"), "title": schemaStr("按标题匹配"), "time": schemaStr("按开始时间匹配 HH:MM")}),
				"changes": schemaObj(map[string]any{"title": schemaStr(""), "time": schemaStr("HH:MM"), "duration_min": schemaInt(""), "type": blockType, "completed": map[string]any{"type": "boolean"}}),
			}, "date", "match", "changes"),
		},
		{
			Name:        "plan_remove",
			Description: "删除指定日期计划里匹配到的时间块。只删这一天；要停掉长期规则用 rule_remove。",
			Parameters: schemaObj(map[string]any{
				"date":  schemaStr("目标日期 YYYY-MM-DD，必须从相对日期对照表取值"),
				"match": schemaObj(map[string]any{"id": schemaStr("块 id"), "title": schemaStr("按标题匹配"), "time": schemaStr("按开始时间匹配 HH:MM")}),
			}, "date", "match"),
		},
		{
			Name:        "rule_upsert",
			Description: "创建或修改长期/重复日程规则（每天/每周几/每月/每 N 天，或远期一次性日期）。带 id 是修改，不带是新建。",
			Parameters: schemaObj(map[string]any{
				"id":           schemaStr("要修改的规则 id（上下文规则列表里有）；新建时省略"),
				"title":        schemaStr("标题（新建必填）"),
				"type":         blockType,
				"kind":         map[string]any{"type": "string", "enum": []string{"recurring", "once"}, "description": "recurring=重复，once=一次性精确日期"},
				"freq":         map[string]any{"type": "string", "enum": []string{"daily", "weekly", "monthly", "every_n_days"}, "description": "kind=recurring 时必填"},
				"interval":     schemaInt("freq=every_n_days 时的 N"),
				"by_weekday":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "freq=weekly 时的周几列表，0=周日…6=周六"},
				"time":         schemaStr("时间 HH:MM，可省略"),
				"duration_min": schemaInt("时长（分钟）"),
				"date":         schemaStr("kind=once 的精确日期 YYYY-MM-DD"),
				"start_date":   schemaStr("重复规则起始日期 YYYY-MM-DD，省略即今天"),
				"until":        schemaStr("重复规则结束日期 YYYY-MM-DD，可省略"),
			}),
		},
		{
			Name:        "rule_remove",
			Description: "删除一条长期/重复规则（用户说\"以后不用了\"）。只是今天不做用 plan_remove。",
			Parameters:  schemaObj(map[string]any{"id": schemaStr("规则 id，来自上下文规则列表")}, "id"),
		},
		{
			Name:        "memory_add",
			Description: "记住一条关于用户的长期事实/偏好（作息、忌口、称呼等）。一次性安排不要存记忆。",
			Parameters:  schemaObj(map[string]any{"fact": schemaStr("简短第三人称陈述，≤50 字，一条只记一件事")}, "fact"),
		},
		{
			Name:        "memory_remove",
			Description: "忘记一条长期记忆。",
			Parameters:  schemaObj(map[string]any{"id": schemaStr("fact id，来自上下文长期记忆列表")}, "id"),
		},
		{
			Name:        "assignment_upsert",
			Description: "记录或修改作业/截止时间（\"下周三交离散\"→新建；\"考试提前到周五\"→带 id 改 due）。不确定用户说的是哪门课时省略 courseName。",
			Parameters: schemaObj(map[string]any{
				"id":         schemaStr("要修改的作业 id（上下文作业列表里有）；新建时省略"),
				"title":      schemaStr("作业/事项标题（新建必填）"),
				"courseId":   schemaStr("课程 id（上下文课程列表里有，最可靠）"),
				"courseName": schemaStr("课程名，不知道 id 时给名字即可"),
				"dueAt":      schemaStr("截止时间，YYYY-MM-DD 或 YYYY-MM-DDTHH:MM；必须从相对日期对照表推算"),
			}),
		},
		{
			Name:        "wish_add",
			Description: "把未定时间的意图投进愿望池（\"想学 Rust\"\"改天去看展\"\"哦对要买牛奶\"）。顺手估个耗时；写前会想一遍是否已经记过——重复的愿望不用再记。",
			Parameters: schemaObj(map[string]any{
				"text":      schemaStr("愿望内容，简短"),
				"effortMin": schemaInt("预估耗时（分钟），用于填缝匹配"),
			}, "text"),
		},
		{
			Name:        "mood_record",
			Description: "从对话里察觉到的情绪直接替用户打卡（\"累死了\"→tired）。映射最接近的一种，原话要点入 note。用户今天已手动打卡过就不要再记（工具会告诉你 alreadyCheckedIn）。",
			Parameters: schemaObj(map[string]any{
				"mood": map[string]any{"type": "string", "enum": moodIDs, "description": "最接近的一种心情"},
				"note": schemaStr("触发原话的要点，≤50 字"),
			}, "mood"),
		},
		{
			Name:        "material_add",
			Description: "归档有长期参考价值的外部知识（考试重点、教室位置、图书馆哪层安静）。正文可长，不受 50 字限制。关于用户自己的规律/偏好用 memory_add，别用错。",
			Parameters: schemaObj(map[string]any{
				"title":    schemaStr("资料标题"),
				"body":     schemaStr("资料正文，可长"),
				"category": schemaStr("类别 id（上下文资料类别列表里有）；不确定可省略"),
			}, "title", "body"),
		},
		{
			Name:        "list_upcoming",
			Description: "查看未来几天的计划与临期作业总览（回答\"这周怎么样/有空吗\"之类问题前先看）。",
			Parameters:  schemaObj(map[string]any{"days": schemaInt("天数 1-14，默认 7")}),
		},
	}
	// 天气：一个源都没有就不进工具带。
	if len(weatherIDs) > 0 {
		params := map[string]any{
			// Any place, not just where the user is. The morning brief needs a
			// stored home location because it has no model to ask; this tool has
			// the opposite property, and saying so is what makes it usable for
			// "what's the weather like in Kyoto next week" while planning a trip.
			"location": schemaStr("任意城市/地名，如\"上海\"、\"京都\"。可以问用户所在地以外的地方 —— 比如商量旅行计划时查目的地。"),
			"days":     schemaInt("预报天数 1-7，默认 2"),
		}
		if len(weatherIDs) > 1 {
			// Only offered when there is a choice to make. A single-element enum
			// is a parameter that can only be filled one way — it spends tokens
			// and invites the model to name a source it did not need to think
			// about.
			params["source"] = schemaEnum("数据源；不填由后端按默认顺序挑。指定了就用那个，那个不可用会直接报错、不会悄悄换一个。", weatherIDs)
		}
		// Offered alongside get_weather, and only then: without a weather source
		// there is nothing a stored home would be used FOR, and a tool that
		// records something nothing reads is a promise the product does not keep.
		tools = append(tools, ai.ToolDef{
			Name: "set_home_location",
			Description: "记住用户**常住**的城市（早晚简报查天气用它，简报里没有模型可以问）。" +
				"只在用户说清了自己住哪 / 长期在哪时才用。" +
				"⚠️ 出差、旅行、\"我这周在东京\" 这类**不要**用它 —— 那种情况直接 get_weather(location=\"东京\") 查一次就行。" +
				"把临时去处存成常住，用户接下来一个月每天早上都会收到错的城市的天气。",
			Parameters: schemaObj(map[string]any{
				"location": schemaStr("城市名，如\"上海\"、\"Cambridge, MA\""),
			}, "location"),
		})
		tools = append(tools, ai.ToolDef{
			Name:        "get_weather",
			Description: "查询某地未来几天的天气预报（用户问天气、安排户外活动、或商量出行计划时主动查）。",
			Parameters:  schemaObj(params, "location"),
		})
	}
	if len(searchIDs) > 0 {
		params := map[string]any{
			"query":       schemaStr("搜索关键词"),
			"max_results": schemaInt("结果条数 1-5，默认 3"),
		}
		if len(searchIDs) > 1 {
			params["source"] = schemaEnum("搜索源；不填由后端按默认顺序挑。", searchIDs)
		}
		tools = append(tools, ai.ToolDef{
			Name:        "web_search",
			Description: "联网搜索实时信息（新闻、地点、时效性事实）。常识问题不要用。",
			Parameters:  schemaObj(params, "query"),
		})
	}
	if interactive {
		tools = append(tools, ai.ToolDef{
			Name:        "propose_decision",
			Description: "需要用户在几个方案里拍板时弹出选择卡（仅限：日程冲突、空档推荐、规则确认这类场景）。大多数操作直接执行即可，不要频繁使用。",
			Parameters: schemaObj(map[string]any{
				"title":   schemaStr("卡片标题，如\"日程冲突\""),
				"summary": schemaStr("一句话说明要决定什么"),
				"options": map[string]any{"type": "array", "items": schemaObj(map[string]any{"id": schemaStr("选项 id"), "label": schemaStr("选项文案")}, "id", "label"), "description": "2-4 个选项"},
			}, "title", "options"),
		})
	}
	return tools
}
