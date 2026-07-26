package domain

// CategoryNote is the always-on fallback material category; every capture that
// fits nothing else lands here, and it can never be disabled.
const CategoryNote = "note"

// MaterialCategory describes one typed material module. The registry is
// code-level (adding a category = one entry here + a classifier hint); whether
// a session has it enabled lives in the session preferences
// (SessionPrefs.MaterialCategories — missing entry means DefaultOn).
type MaterialCategory struct {
	ID         string `json:"id"`
	NameZH     string `json:"-"`
	NameEN     string `json:"-"`
	Icon       string `json:"icon"`    // frontend icon hint
	PromptHint string `json:"-"`       // one-line guidance for the inbox AI classifier
	DefaultOn  bool   `json:"default"` // enabled unless the user opted out
}

var materialCategories = []MaterialCategory{
	{ID: CategoryNote, NameZH: "通用", NameEN: "Notes", Icon: "note", DefaultOn: true,
		PromptHint: "其它类别都不合适时的通用笔记/备忘。"},
	{ID: "diet", NameZH: "饮食", NameEN: "Diet", Icon: "utensils", DefaultOn: true,
		PromptHint: "吃了什么、饮食记录、热量/营养（如：中午吃了牛肉面大概600大卡）。"},
	{ID: "health", NameZH: "健康", NameEN: "Health", Icon: "heart-pulse", DefaultOn: true,
		PromptHint: "症状、体征、用药、体检报告（如：今天头有点疼、血压 120/80）。"},
	{ID: "academic", NameZH: "学业", NameEN: "Academic", Icon: "graduation-cap", DefaultOn: true,
		PromptHint: "课程、作业、考试、截止日期（如：数分作业周五截止）。"},
	{ID: "travel", NameZH: "出行", NameEN: "Travel", Icon: "plane", DefaultOn: true,
		PromptHint: "旅行计划、目的地、行程、交通住宿（如：八月想去东京玩五天）。"},
	{ID: "finance", NameZH: "财务", NameEN: "Finance", Icon: "wallet", DefaultOn: false,
		PromptHint: "消费、账单、预算、报销（如：今天花了 45 买午饭）。"},
	{ID: "fitness", NameZH: "运动", NameEN: "Fitness", Icon: "dumbbell", DefaultOn: false,
		PromptHint: "锻炼记录、运动计划、体测数据（如：晚上跑了 5 公里）。"},
	{ID: "idea", NameZH: "灵感", NameEN: "Ideas", Icon: "lightbulb", DefaultOn: false,
		PromptHint: "想法、灵感、待研究的主题（如：也许可以做一个自动记账的小工具）。"},
	{ID: "shopping", NameZH: "购物", NameEN: "Shopping", Icon: "shopping-cart", DefaultOn: false,
		PromptHint: "想买的东西、购物清单、比价（如：记得买洗衣液）。"},
	{ID: "media", NameZH: "书影音", NameEN: "Media", Icon: "film", DefaultOn: false,
		PromptHint: "想看/看过的书、电影、剧、音乐、游戏及观后感（如：朋友推荐了一部电影）。"},
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
