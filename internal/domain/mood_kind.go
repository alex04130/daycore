package domain

import "daycore/internal/i18n"

// MoodKind is one of the twelve check-in moods. The set, the order, and the
// emoji are copied from the design prototype (design-ui/core/daycore-core.js
// MOODS) — that is the list the four frontends draw, so it is a product
// decision rather than an implementation detail.
//
// Stored values are these stable ids, not the display labels: the Chinese label
// is what a user sees, but persisting it would make renaming a mood a data
// migration and would leave the English UI storing different rows for the same
// feeling. The same reasoning is why Names is a locale map and not a pair of
// fields — a third language should be new entries, not a new column.
type MoodKind struct {
	ID    string    `json:"id"`
	Emoji string    `json:"emoji"`
	Names i18n.Text `json:"-"`
	// Valence is how good or bad this feels, −2..+2. It exists so a run of
	// check-ins can be read as a trend; it is never shown to the user, and it
	// is deliberately coarse — the difference between 疲惫 and 压力大 is a
	// difference in kind, and pretending to rank them finely would be false
	// precision.
	Valence int `json:"-"`
}

func moodNames(zh, en string) i18n.Text { return i18n.Text{"zh-CN": zh, "en-US": en} }

var moodKinds = []MoodKind{
	{ID: "happy", Emoji: "😊", Names: moodNames("开心", "Happy"), Valence: 2},
	{ID: "calm", Emoji: "😌", Names: moodNames("平静", "Calm"), Valence: 1},
	{ID: "loved", Emoji: "🥰", Names: moodNames("被爱", "Loved"), Valence: 2},
	{ID: "excited", Emoji: "🤩", Names: moodNames("兴奋", "Excited"), Valence: 2},
	{ID: "neutral", Emoji: "😐", Names: moodNames("一般", "Neutral"), Valence: 0},
	{ID: "tired", Emoji: "😪", Names: moodNames("疲惫", "Tired"), Valence: -1},
	{ID: "stressed", Emoji: "😣", Names: moodNames("压力大", "Stressed"), Valence: -2},
	{ID: "anxious", Emoji: "😟", Names: moodNames("焦虑", "Anxious"), Valence: -2},
	{ID: "down", Emoji: "😢", Names: moodNames("低落", "Down"), Valence: -2},
	{ID: "irritable", Emoji: "😠", Names: moodNames("烦躁", "Irritable"), Valence: -1},
	{ID: "unwell", Emoji: "🤒", Names: moodNames("不舒服", "Unwell"), Valence: -1},
	{ID: "sleepless", Emoji: "🌙", Names: moodNames("失眠", "Sleepless"), Valence: -1},
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

// MoodName returns the label for a locale, using the shared fallback chain
// (i18n.Pick): exact, then the same language in another region, then en-US.
//
// Falling back to en-US rather than to zh-CN — the design original — matches
// what i18n.Resolve already does to an unrecognised Accept-Language, so a
// caller that goes through the normal negotiation and one that passes a raw tag
// land in the same place.
func (k MoodKind) MoodName(locale string) string { return i18n.Pick(k.Names, locale) }

// Mood sources. An agent-recorded check-in is an inference from what the user
// said; a user one is the user pressing a button. Both are real, but they do
// not carry the same weight and the user must be able to see which is which
// (EXPERIENCE_CORE §12.1).
const (
	MoodSourceUser  = "user"
	MoodSourceAgent = "agent"
)
