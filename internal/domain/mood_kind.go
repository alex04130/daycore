package domain

// MoodKind is one of the twelve check-in moods. The set, the order, and the
// emoji are copied from the design prototype (design-ui/core/daycore-core.js
// MOODS) — that is the list the four frontends draw, so it is a product
// decision rather than an implementation detail.
//
// Stored values are these stable ids, not the display labels: the Chinese label
// is what a user sees, but persisting it would make renaming a mood a data
// migration and would leave the English UI storing different rows for the same
// feeling.
type MoodKind struct {
	ID     string `json:"id"`
	Emoji  string `json:"emoji"`
	NameZH string `json:"-"`
	NameEN string `json:"-"`
	// Valence is how good or bad this feels, −2..+2. It exists so a run of
	// check-ins can be read as a trend; it is never shown to the user, and it
	// is deliberately coarse — the difference between 疲惫 and 压力大 is a
	// difference in kind, and pretending to rank them finely would be false
	// precision.
	Valence int `json:"-"`
}

var moodKinds = []MoodKind{
	{ID: "happy", Emoji: "😊", NameZH: "开心", NameEN: "Happy", Valence: 2},
	{ID: "calm", Emoji: "😌", NameZH: "平静", NameEN: "Calm", Valence: 1},
	{ID: "loved", Emoji: "🥰", NameZH: "被爱", NameEN: "Loved", Valence: 2},
	{ID: "excited", Emoji: "🤩", NameZH: "兴奋", NameEN: "Excited", Valence: 2},
	{ID: "neutral", Emoji: "😐", NameZH: "一般", NameEN: "Neutral", Valence: 0},
	{ID: "tired", Emoji: "😪", NameZH: "疲惫", NameEN: "Tired", Valence: -1},
	{ID: "stressed", Emoji: "😣", NameZH: "压力大", NameEN: "Stressed", Valence: -2},
	{ID: "anxious", Emoji: "😟", NameZH: "焦虑", NameEN: "Anxious", Valence: -2},
	{ID: "down", Emoji: "😢", NameZH: "低落", NameEN: "Down", Valence: -2},
	{ID: "irritable", Emoji: "😠", NameZH: "烦躁", NameEN: "Irritable", Valence: -1},
	{ID: "unwell", Emoji: "🤒", NameZH: "不舒服", NameEN: "Unwell", Valence: -1},
	{ID: "sleepless", Emoji: "🌙", NameZH: "失眠", NameEN: "Sleepless", Valence: -1},
}

var moodByID = func() map[string]MoodKind {
	m := make(map[string]MoodKind, len(moodKinds))
	for _, k := range moodKinds {
		m[k.ID] = k
	}
	return m
}()

// MoodKinds returns the registry in display order.
func MoodKinds() []MoodKind { return append([]MoodKind(nil), moodKinds...) }

// MoodKindByID looks up a mood; ok is false for anything not in the registry.
func MoodKindByID(id string) (MoodKind, bool) {
	k, ok := moodByID[id]
	return k, ok
}

// MoodName returns the label for a locale, falling back to zh-CN — the design
// original — rather than to English.
func (k MoodKind) MoodName(locale string) string {
	if len(locale) >= 2 && (locale[:2] == "en" || locale[:2] == "EN") {
		return k.NameEN
	}
	return k.NameZH
}

// Mood sources. An agent-recorded check-in is an inference from what the user
// said; a user one is the user pressing a button. Both are real, but they do
// not carry the same weight and the user must be able to see which is which
// (EXPERIENCE_CORE §12.1).
const (
	MoodSourceUser  = "user"
	MoodSourceAgent = "agent"
)
