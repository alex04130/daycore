package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// The ladder ends in silence, and that is the answer rather than a gap.
//
// The brief used to ask about 北京 — hardcoded, for every user anywhere. A
// wrong city is worse than no city: "17°C and raining" is a sentence somebody
// dresses by, and being quietly wrong about it every morning erodes trust in
// the four other things the brief says.
func TestNoKnownLocationMeansNoWeather(t *testing.T) {
	s, sid := newAgentTestServer(t)
	place, source := s.SessionLocation(context.Background(), sid)
	if place != "" {
		t.Errorf("a session that never said where it is resolved to %q", place)
	}
	if source != "" {
		t.Errorf("source = %q for an unknown location", source)
	}
}

// The user's own choice outranks whatever the client reports, permanently.
// Somebody who set their home city on purpose — because that is the day they
// plan around — must not have it rewritten the first time they open the app
// from a train.
func TestAClientHintNeverOverwritesTheUsersChoice(t *testing.T) {
	prefs := DefaultPrefs()

	if !applyLocationHint(&prefs, "上海") {
		t.Fatal("a hint was refused for an empty location")
	}
	if prefs.Location != "上海" || prefs.LocationSource != LocSourceDetected {
		t.Fatalf("hint did not land: %q/%q", prefs.Location, prefs.LocationSource)
	}
	// A later hint updates a detected value — the device moved.
	if !applyLocationHint(&prefs, "杭州") || prefs.Location != "杭州" {
		t.Error("a second hint did not update a detected location")
	}
	// The same hint again is not a write.
	if applyLocationHint(&prefs, "杭州") {
		t.Error("an unchanged hint reported a change, which would write on every request")
	}

	prefs.Location, prefs.LocationSource = "北京", LocSourceUser
	if applyLocationHint(&prefs, "东京") {
		t.Error("a client hint overwrote the user's own choice")
	}
	if prefs.Location != "北京" {
		t.Errorf("location is now %q", prefs.Location)
	}
}

func TestOversizedHintsAreRefused(t *testing.T) {
	prefs := DefaultPrefs()
	if applyLocationHint(&prefs, strings.Repeat("城", 200)) {
		t.Error("a 200-character 'place' was accepted")
	}
	if applyLocationHint(&prefs, "   ") {
		t.Error("whitespace was accepted as a location")
	}
}

// Rung two reads memory, and reads it conservatively. The cost of being wrong
// is a confidently wrong forecast every morning; the cost of falling through is
// one missing line. With costs that lopsided the parser should be boring.
func TestMemoryExtractionFiresOnlyOnAStatedHome(t *testing.T) {
	cases := []struct{ fact, want string }{
		{"我住在上海，喜欢下雨天", "上海"},
		{"现居杭州", "杭州"},
		{"lives in Cambridge, MA", "Cambridge"},
		{"location: Berlin", "Berlin"},
		{"常住北京。周末回天津", "北京"},

		// Falls through: these are not statements about where somebody lives.
		{"想去京都看樱花", ""},
		{"下周要飞东京出差", ""},
		{"喜欢北京的秋天", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := locationFromFacts([]domain.MemoryFact{{Fact: tc.fact}})
		if got != tc.want {
			t.Errorf("%q → %q, want %q", tc.fact, got, tc.want)
		}
	}
}

// A marker inside a long sentence is a sentence, not a place. Handing a
// sentence to a weather source gets an answer for whatever it manages to match,
// which is the confidently wrong forecast this file exists to avoid.
func TestALongFragmentIsNotAPlace(t *testing.T) {
	long := "我在" + strings.Repeat("很", 80) + "远的地方"
	if got := locationFromFacts([]domain.MemoryFact{{Fact: long}}); got != "" {
		t.Errorf("a %d-rune fragment was treated as a place: %q", len([]rune(long)), got)
	}
}

// The explicit setting outranks memory: a fact recorded in passing is weaker
// evidence than a field somebody filled in, and a stale one ("I'm in Tokyo this
// week") must not beat a current setting.
func TestTheSettingOutranksMemory(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	if _, err := s.store.Memory().AddFact(ctx, &domain.MemoryFact{SessionID: sid, Fact: "我住在东京"}); err != nil {
		t.Fatal(err)
	}
	if place, source := s.SessionLocation(ctx, sid); place != "东京" || source != LocSourceMemory {
		t.Fatalf("memory rung: %q/%q", place, source)
	}

	prefs := s.sessionPrefs(ctx, sid)
	prefs.Location, prefs.LocationSource = "北京", LocSourceUser
	saveTestPrefs(t, s, sid, prefs)

	place, source := s.SessionLocation(ctx, sid)
	if place != "北京" || source != LocSourceUser {
		t.Errorf("the settings page lost to a memory fact: %q/%q", place, source)
	}
}

func saveTestPrefs(t *testing.T, s *Server, sid string, prefs SessionPrefs) {
	t.Helper()
	raw, err := json.Marshal(prefs)
	if err != nil {
		t.Fatal(err)
	}
	str := string(raw)
	if _, err := s.store.Sessions().Update(context.Background(), sid, domain.SessionUpdate{Preferences: &str}); err != nil {
		t.Fatal(err)
	}
}
