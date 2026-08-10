package theme

// The six primitive kinds every build ships with.
//
// # Why these are compiled in as well as being data
//
// The three layers exist so that ADDING a kind needs no release. Removing one
// must still be impossible, because every stored theme was validated against
// the kind its token declared — and a deployment whose `color` disappeared
// (a bad file, an empty table) would reject every theme write with "unknown
// kind", which reads exactly like data loss.
//
// So this is the floor: merged first, always present, and overridable by a
// later layer but never removable. Same arrangement as the embedded zh-CN /
// en-US message catalogue.
//
// # Every pattern here is deliberately NARROWER than CSS allows
//
// CSS accepts far more than this. That is fine: a theme token is a value a
// stylesheet interpolates, so the question is not "what is legal CSS" but "what
// is the smallest alphabet that still expresses a theme". Anything genuinely
// missing is a combinator away, or a third-tier pattern an operator approved.
func EmbeddedKinds() []Kind {
	return []Kind{
		{
			Name: "color",
			// Hex, the four functional notations, and the two keywords a theme
			// realistically needs. ⚠️ NOT `currentColor`, `inherit` or a named
			// colour: those resolve against context, so the same theme would
			// look different depending on where it landed — which is the one
			// thing a theme must not do.
			// ⚠️ {3,8} would be wrong: CSS hex is 3, 4, 6 or 8 digits, and five or
			// seven is a typo the browser silently ignores — so accepting it
			// would store a theme that renders as "no colour at all" with
			// nothing anywhere reporting why.
			Pattern:     `#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})|(?:rgb|rgba|hsl|hsla)\([0-9.,%\s/deg]+\)|transparent|none`,
			Description: "颜色：#hex、rgb()/rgba()/hsl()/hsla()、transparent、none",
		},
		{
			Name: "length",
			// A whitelist of units rather than "any identifier": `pt`, `cm` and
			// friends are meaningless on a screen, and `q`/`ic`/`lh` are the
			// kind of thing that makes a layout depend on the font.
			Pattern:     `-?[0-9]*\.?[0-9]+(?:px|rem|em|ch|vh|vw|vmin|vmax|%)|0`,
			Description: "长度：数字加 px/rem/em/ch/vh/vw/vmin/vmax/%，或裸 0",
		},
		{
			Name:        "number",
			Pattern:     `-?[0-9]*\.?[0-9]+`,
			Description: "纯数字，不带单位",
		},
		{
			Name: "ratio",
			// 0..1 inclusive. Expressed as a pattern rather than a numeric range
			// because the whole system is regex-shaped and one exception would
			// mean two ways a kind can be defined.
			Pattern:     `0|1|0?\.[0-9]+|1\.0+`,
			Description: "0 到 1 之间的比例，例如 0.72",
		},
		{
			Name:        "duration",
			Pattern:     `[0-9]*\.?[0-9]+(?:ms|s)`,
			Description: "时长：数字加 ms 或 s",
		},
		{
			Name: "enum",
			// The bare `enum` accepts a plain identifier. A manifest that wants
			// a SPECIFIC set writes `one-of[a,b,c]` instead, which is checked
			// against the literals — this one only says "an identifier, not an
			// expression".
			Pattern:     `[a-zA-Z][a-zA-Z0-9_-]{0,63}`,
			Description: "一个标识符。要限定具体取值请用 one-of[a,b,c]",
		},
	}
}
