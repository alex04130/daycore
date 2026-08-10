package server

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"daycore/internal/domain"
)

// Where the briefs look up the weather.
//
// # The ladder, and why its last rung is silence
//
//  1. the client said so            the app knows where the device is
//  2. the user's long-term memory   they told the assistant at some point
//  3. nothing                       the brief has no weather line
//
// The third rung is an answer, not a gap waiting to be filled. Before this, the
// brief asked about 北京 — hardcoded, for every user anywhere, with
// `// future: session setting` written beside it. **A wrong city is worse than
// no city**: "17°C and raining" is a sentence somebody dresses by, and being
// quietly wrong about it every morning erodes trust in the four other things
// the brief says. Saying nothing costs one line.
//
// That judgement is also why rung 2 is conservative rather than clever — see
// locationFromFacts.
//
// # Why the model is not on this path at all
//
// A brief is a Chat with no Tools attached, so there is no tool band and
// nothing to ask. The tool path needs none of this: get_weather takes a
// location as an argument, so the model can already ask about anywhere — the
// trip being planned, the city somebody is flying to next week. That is a
// different question from "where is this person right now", and only the second
// one needs an answer stored on the session.
const (
	// LocSourceUser is a place the user typed on the settings page.
	LocSourceUser = "user"
	// LocSourceDetected is a place the client reported. A later hint may replace
	// it; it may never replace LocSourceUser.
	LocSourceDetected = "detected"
	// LocSourceMemory is never stored. It is what a lookup reports when the
	// place came from a memory fact, so a caller can weigh how sure to be.
	LocSourceMemory = "memory"
)

// SessionLocation resolves where a user is, and says how it knows.
//
// Returns ("", "") when nothing is known. Callers must read that as "no
// weather", never as permission to guess.
func (s *Server) SessionLocation(ctx context.Context, sid string) (place, source string) {
	prefs := s.sessionPrefs(ctx, sid)
	if p := strings.TrimSpace(prefs.Location); p != "" {
		src := prefs.LocationSource
		if src == "" {
			src = LocSourceDetected
		}
		return p, src
	}
	if s.store == nil {
		return "", ""
	}
	// The memory rung is read only after the explicit setting. A fact the
	// assistant recorded in passing is weaker evidence than a field the user
	// filled in, and a stale one ("I'm in Tokyo this week") must not outrank a
	// current setting.
	facts, err := s.store.Memory().ListFacts(ctx, sid)
	if err != nil {
		return "", ""
	}
	if p := locationFromFacts(facts); p != "" {
		return p, LocSourceMemory
	}
	return "", ""
}

// locationMarkers are the phrases that make a fact a statement about where
// somebody lives. Each is followed by the place.
//
// An explicit list rather than a general parser, because the alternative is
// guessing: memory facts are free text written by the model in the user's own
// words, and any heuristic loose enough to catch most of them is loose enough
// to pull a city out of "I want to visit Kyoto". This fires on a stated home
// and falls through on everything else.
var locationMarkers = []string{
	"我住在", "我在", "常住", "所在城市是", "所在地是", "现居",
	"lives in ", "live in ", "based in ", "location: ", "city: ",
}

// locationFromFacts extracts a home location, or "" when it is not sure.
//
// # Boundary: conservative on purpose, and it falls through rather than guess
//
// It only fires on an explicit marker followed by something short enough to be
// a place name. It deliberately does NOT try to understand a sentence, because
// the cost of being wrong here is a confidently wrong forecast every morning,
// and the cost of falling through is one missing line. When the two costs are
// that lopsided, the parser should be the boring one.
//
// If somebody wants this to be reliable, the answer is the settings page (rung
// 1), not a better regex.
func locationFromFacts(facts []domain.MemoryFact) string {
	for _, f := range facts {
		text := strings.TrimSpace(f.Fact)
		lower := strings.ToLower(text)
		for _, marker := range locationMarkers {
			i := strings.Index(lower, strings.ToLower(marker))
			if i < 0 {
				continue
			}
			rest := strings.TrimSpace(text[i+len(marker):])
			if p := firstClause(rest); p != "" {
				return p
			}
		}
	}
	return ""
}

// firstClause takes the place name off the front of a fragment and stops at the
// first punctuation, so "北京，喜欢下雨天" yields "北京".
//
// Bounded at 64 characters: anything longer is a sentence that happened to
// contain a marker, not a place, and handing a sentence to a weather source
// gets an answer for whatever it manages to match — which is the confidently
// wrong forecast this whole file is arranged to avoid.
func firstClause(s string) string {
	stop := strings.IndexFunc(s, func(r rune) bool {
		switch r {
		case '，', ',', '。', '.', ';', '；', '\n', '(', '（':
			return true
		}
		return false
	})
	if stop >= 0 {
		s = s[:stop]
	}
	s = strings.TrimFunc(strings.TrimSpace(s), func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	if s == "" || len([]rune(s)) > 64 {
		return ""
	}
	return s
}

// applyLocationHint records a client-reported place.
//
// Same rule as the timezone hint, and the same reason: it may fill in an empty
// value or update a previously detected one, but it must never overwrite what
// the user typed. Somebody who set their home city deliberately — because that
// is the day they plan around — should not have it rewritten the first time
// they open the app from somewhere else.
func applyLocationHint(prefs *SessionPrefs, hint string) bool {
	hint = strings.TrimSpace(hint)
	if hint == "" || len([]rune(hint)) > 128 {
		return false
	}
	if prefs.LocationSource == LocSourceUser {
		return false
	}
	if prefs.Location == hint && prefs.LocationSource == LocSourceDetected {
		return false
	}
	prefs.Location, prefs.LocationSource = hint, LocSourceDetected
	return true
}

// noteClientLocation records a place the client reported.
//
// Best-effort and never fails a request: this runs on paths that have their own
// job to do, and a session whose location could not be saved is exactly as
// broken as it was a moment ago.
//
// Unlike the timezone hint, this does NOT reschedule anything — a location
// changes what a brief says, not when it fires. That asymmetry is why the two
// live in separate files despite the identical precedence rule.
func (s *Server) noteClientLocation(ctx context.Context, sid, hint string) {
	if s == nil || s.store == nil || sid == "" {
		return
	}
	prefs := s.sessionPrefs(ctx, sid)
	if !applyLocationHint(&prefs, hint) {
		return
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return
	}
	str := string(raw)
	if _, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{Preferences: &str}); err != nil {
		s.log.Debug("could not store the session location", "sid", sid, "err", err)
	}
}
