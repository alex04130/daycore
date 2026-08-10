package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"daycore/internal/domain"
)

func init() {
	registerRevert("home_location_set", (*Server).revertHomeLocation)
}

// toolSetHomeLocation records where the user lives, from the conversation.
//
// # Why this tool exists at all
//
// The briefs need a place and have no model to ask — they are a Chat with no
// Tools attached. Something has to put a place on the session, and the only
// other doors are a settings page and a client hint. Both are worse:
//
//   - **The settings page is friction nobody spends.** People do not open
//     settings to fill in a field for a feature they have not seen work yet,
//     and this one is chicken-and-egg: the weather line only appears after the
//     location exists, so nothing ever prompts them to go and set it.
//   - **The client cannot report a city.** A browser gives a timezone for free
//     (the frontend already sends it, so that half needs nobody's attention)
//     but a place name needs either a permission prompt returning coordinates,
//     or a third-party IP lookup. Coordinates would mean owning a geocoder
//     here; an IP lookup means a network call about the user to somebody else.
//
// A conversation is the app's native surface. "我在上海" is a thing people say
// unprompted, and this is what makes the assistant remember it properly.
//
// # The distinction this tool is built around
//
//	stored location   where they LIVE — used by the briefs, which cannot ask
//	tool argument     anywhere at all — the trip, next week's city, a friend's town
//
// get_weather takes a location argument, so asking about somewhere else needs
// nothing stored. Conflating the two is the failure this tool's description
// spends its words preventing: somebody saying "我这周在东京" wants a forecast,
// not their home rewritten, and a home quietly reset by a business trip is a
// wrong weather line every morning for the following month.
//
// # Boundary: this writes the same field the settings page writes
//
// Source is user, because the user said it — just out loud rather than into a
// form. That means a later client hint cannot overwrite it, which is right for
// a home and is exactly why the description insists it only be used for one.
func (s *Server) toolSetHomeLocation(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments")
	}
	place := strings.TrimSpace(args.Location)
	if place == "" {
		return toolFail("location is required")
	}
	if len([]rune(place)) > 128 {
		return toolFail("that is a sentence, not a place name")
	}

	prefs := s.sessionPrefs(ctx, sid)
	before := prefs.Location
	if before == place && prefs.LocationSource == LocSourceUser {
		// Saying it again is not a change. Returning ok without an op keeps the
		// undo log free of entries that would undo nothing — a revert that
		// restores the same value is noise in the one place a person scrolls
		// looking for what actually happened.
		return toolResult{OK: true, Data: map[string]any{"location": place, "unchanged": true}, Summary: place}
	}
	prefs.Location, prefs.LocationSource = place, LocSourceUser
	if err := s.saveSessionPrefs(ctx, sid, prefs); err != nil {
		return toolFail("could not save the location: %v", err)
	}

	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "home_location_set",
		Summary: place,
		Detail:  marshalCompact(map[string]any{"before": before, "after": place}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"location": place}, Summary: place}
}

// revertHomeLocation puts back whatever was there before, including nothing.
//
// Restoring "" has to work and has to clear the source with it: a session that
// had no location must go back to having none, or an undo leaves the user
// pinned to a place they never set with a source that says they chose it.
func (s *Server) revertHomeLocation(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	before, _ := detail.Before.(string)
	prefs := s.sessionPrefs(ctx, sid)
	prefs.Location = before
	if before == "" {
		prefs.LocationSource = ""
	} else {
		prefs.LocationSource = LocSourceUser
	}
	_ = s.saveSessionPrefs(ctx, sid, prefs)
	s.finishRevert(ctx, w, sid, orig)
}

// saveSessionPrefs persists a preferences struct.
//
// Extracted because three paths now write it — the settings handler, the client
// hint, and this tool — and the marshal-then-Update dance is the kind of thing
// that acquires a subtle difference in one of three copies.
func (s *Server) saveSessionPrefs(ctx context.Context, sid string, prefs SessionPrefs) error {
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	str := string(raw)
	_, err = s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{Preferences: &str})
	return err
}
