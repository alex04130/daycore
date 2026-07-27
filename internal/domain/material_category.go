package domain

import "daycore/internal/i18n"

// CategoryNote is the always-on fallback material category; every capture that
// fits nothing else lands here, and it can never be disabled.
const CategoryNote = "note"

// MaterialCategory describes one typed material module. The registry is
// code-level (adding a category = one entry here + a classifier hint); whether
// a session has it enabled lives in the session preferences
// (SessionPrefs.MaterialCategories — missing entry means DefaultOn).
type MaterialCategory struct {
	ID    string    `json:"id"`
	Names i18n.Text `json:"-"`
	Icon  string    `json:"icon"` // frontend icon hint
	// Hints is one line of guidance per locale for the inbox AI classifier. It
	// is locale-keyed for the same reason the prompt templates are: a hint
	// written in Chinese inside an English prompt is a language switch mid-
	// instruction, which is exactly the kind of thing that makes a classifier
	// answer in the wrong language.
	Hints     i18n.Text `json:"-"`
	DefaultOn bool      `json:"default"` // enabled unless the user opted out
}

// Name returns the display label for a locale (see i18n.Pick for the chain).
func (c MaterialCategory) Name(locale string) string { return i18n.Pick(c.Names, locale) }

// Hint returns the classifier guidance for a locale.
func (c MaterialCategory) Hint(locale string) string { return i18n.Pick(c.Hints, locale) }

func catNames(zh, en string) i18n.Text { return i18n.Text{"zh-CN": zh, "en-US": en} }

var materialCategories = []MaterialCategory{
	{ID: CategoryNote, Names: catNames("通用", "Notes"), Icon: "note", DefaultOn: true,
		Hints: i18n.Text{
			"zh-CN": "其它类别都不合适时的通用笔记/备忘。",
			"en-US": "General notes and reminders — anything no other category fits.",
		}},
	{ID: "diet", Names: catNames("饮食", "Diet"), Icon: "utensils", DefaultOn: true,
		Hints: i18n.Text{
			"zh-CN": "吃了什么、饮食记录、热量/营养（如：中午吃了牛肉面大概600大卡）。",
			"en-US": "Meals, food logs, calories and nutrition (e.g. \"had a beef noodle soup for lunch, maybe 600 kcal\").",
		}},
	{ID: "health", Names: catNames("健康", "Health"), Icon: "heart-pulse", DefaultOn: true,
		Hints: i18n.Text{
			"zh-CN": "症状、体征、用药、体检报告（如：今天头有点疼、血压 120/80）。",
			"en-US": "Symptoms, vitals, medication, check-up results (e.g. \"slight headache today\", \"BP 120/80\").",
		}},
	{ID: "academic", Names: catNames("学业", "Academic"), Icon: "graduation-cap", DefaultOn: true,
		Hints: i18n.Text{
			"zh-CN": "课程、作业、考试、截止日期（如：数分作业周五截止）。",
			"en-US": "Courses, assignments, exams, deadlines (e.g. \"calculus homework due Friday\").",
		}},
	{ID: "travel", Names: catNames("出行", "Travel"), Icon: "plane", DefaultOn: true,
		Hints: i18n.Text{
			"zh-CN": "旅行计划、目的地、行程、交通住宿（如：八月想去东京玩五天）。",
			"en-US": "Trip plans, destinations, itineraries, transport and lodging (e.g. \"five days in Tokyo in August\").",
		}},
	{ID: "finance", Names: catNames("财务", "Finance"), Icon: "wallet", DefaultOn: false,
		Hints: i18n.Text{
			"zh-CN": "消费、账单、预算、报销（如：今天花了 45 买午饭）。",
			"en-US": "Spending, bills, budgets, reimbursements (e.g. \"spent 45 on lunch today\").",
		}},
	{ID: "fitness", Names: catNames("运动", "Fitness"), Icon: "dumbbell", DefaultOn: false,
		Hints: i18n.Text{
			"zh-CN": "锻炼记录、运动计划、体测数据（如：晚上跑了 5 公里）。",
			"en-US": "Workout logs, training plans, fitness measurements (e.g. \"ran 5 km this evening\").",
		}},
	{ID: "idea", Names: catNames("灵感", "Ideas"), Icon: "lightbulb", DefaultOn: false,
		Hints: i18n.Text{
			"zh-CN": "想法、灵感、待研究的主题（如：也许可以做一个自动记账的小工具）。",
			"en-US": "Ideas, sparks, topics to look into later (e.g. \"maybe build a little expense-tracking tool\").",
		}},
	{ID: "shopping", Names: catNames("购物", "Shopping"), Icon: "shopping-cart", DefaultOn: false,
		Hints: i18n.Text{
			"zh-CN": "想买的东西、购物清单、比价（如：记得买洗衣液）。",
			"en-US": "Things to buy, shopping lists, price comparisons (e.g. \"remember to get laundry detergent\").",
		}},
	{ID: "media", Names: catNames("书影音", "Media"), Icon: "film", DefaultOn: false,
		Hints: i18n.Text{
			"zh-CN": "想看/看过的书、电影、剧、音乐、游戏及观后感（如：朋友推荐了一部电影）。",
			"en-US": "Books, films, shows, music and games — watched, read or on the list, plus reactions (e.g. \"a friend recommended a film\").",
		}},
}

// MaterialCategories returns the full registry in display order (copy — safe
// for callers to keep).
func MaterialCategories() []MaterialCategory {
	out := make([]MaterialCategory, len(materialCategories))
	copy(out, materialCategories)
	return out
}

// MaterialCategoryByID looks a category up by id.
func MaterialCategoryByID(id string) (MaterialCategory, bool) {
	for _, c := range materialCategories {
		if c.ID == id {
			return c, true
		}
	}
	return MaterialCategory{}, false
}
