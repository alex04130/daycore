package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

func (s *Server) toolGetWeather(ctx context.Context, sid, locale, rawArgs string) toolResult {
	var args struct {
		Location string `json:"location"`
		Days     int    `json:"days"`
		// Source is optional. Omitted means "pick for me", which walks the
		// configured order and takes the first usable one. Named means exactly
		// that one — a named source that is down is an ERROR, never a quiet
		// substitution: answering with another source's data under the same
		// question is the cross-source chain this batch deleted, and the model
		// has no way to notice it happened.
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || strings.TrimSpace(args.Location) == "" {
		return toolFail("location is required")
	}
	if s.weather == nil || !s.weather.Available() {
		return toolFail("weather is not configured")
	}
	f, err := s.weather.Lookup(ctx, args.Source, domain.WeatherQuery{Location: args.Location, Days: args.Days, Locale: locale})
	if err != nil {
		// The error text is the adapter package's model-facing one: source, kind
		// and status, never the URL. An adapter's own words routinely quote the
		// address it was called on, and a tool failure travels into the model's
		// context and from there into what the assistant says out loud.
		return toolFail("weather lookup failed: %v", err)
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "tool_get_weather", Summary: args.Location,
	})
	return toolResult{OK: true, Data: map[string]any{"forecast": f, "summary": f.Summary(locale)}, Summary: args.Location}
}

func (s *Server) toolWebSearch(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
		Source     string `json:"source"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || strings.TrimSpace(args.Query) == "" {
		return toolFail("query is required")
	}
	if args.MaxResults <= 0 {
		args.MaxResults = 3
	}
	if s.search == nil || !s.search.Available() {
		return toolFail("web search is not configured")
	}
	results, err := s.search.Search(ctx, args.Source, args.Query, args.MaxResults)
	if err != nil {
		return toolFail("search failed: %v", err)
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "tool_web_search", Summary: args.Query,
	})
	// Snippets are the first third-party text that reaches the model, so they
	// cross the untrusted gate: the client sees the clean results while the
	// model gets the same bytes wrapped as data-not-instructions.
	return toolResult{
		OK: true, Data: map[string]any{"results": results}, Summary: args.Query,
		ForModel: untrustedWrap("联网搜索", marshalCompact(map[string]any{"results": results})),
	}
}

func (s *Server) toolListUpcoming(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Days int `json:"days"`
	}
	_ = json.Unmarshal([]byte(rawArgs), &args)
	if args.Days <= 0 {
		args.Days = 7
	}
	if args.Days > 14 {
		args.Days = 14
	}
	today := time.Now().Format("2006-01-02")
	start, _ := time.Parse("2006-01-02", today)
	days := []map[string]any{}
	for i := 0; i < args.Days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		blocks := s.planBlocksForDate(ctx, sid, date)
		if len(blocks) == 0 {
			continue
		}
		items := make([]map[string]any, 0, len(blocks))
		for _, b := range blocks {
			item := map[string]any{"title": b.Title, "type": b.Type, "completed": b.Completed}
			if b.Time != nil {
				item["time"] = *b.Time
			}
			items = append(items, item)
		}
		days = append(days, map[string]any{"date": date, "blocks": items})
	}
	assignments := assignmentSummaries(s.upcomingAssignments(ctx, sid, args.Days))
	return toolResult{OK: true,
		Data:    map[string]any{"days": days, "assignments": assignments},
		Summary: fmt.Sprintf("%d days", args.Days)}
}

// weatherIDs and searchIDs are the usable source ids for the tool band, sorted.
// nil-safe because a Server can legitimately be built without either — a
// deployment with no weather simply does not offer the tool.
func (s *Server) weatherIDs() []string {
	if s.weather == nil {
		return nil
	}
	return s.weather.IDs()
}

func (s *Server) searchIDs() []string {
	if s.search == nil {
		return nil
	}
	return s.search.IDs()
}

// sourceLines is the per-turn description block for L3.
//
// # Everything here has been through the approval gate
//
// Source.PromptDescription returns the operator's text only when it has been
// approved against a hash of those exact words, and otherwise a sentence built
// from the id and format. An adapter's own self-reported description cannot
// reach this function — see internal/adapters/source.go and the two structural
// gates that make removing that separation something a person has to do on
// purpose.
//
// # It is built from the same set as the tool enum, in the same round
//
// Describing a source the model cannot call is worse than describing nothing:
// it invites a call that fails and spends a round trip on it. Both come from
// IDs()/Describe() and both are read once per round.
func (s *Server) sourceLines(locale string) []ai.SourceLine {
	out := []ai.SourceLine{}
	if s.weather != nil {
		for _, d := range s.weather.Describe(locale) {
			out = append(out, ai.SourceLine{Tool: "get_weather", ID: d[0], Text: d[1]})
		}
	}
	if s.search != nil {
		for _, d := range s.search.Describe(locale) {
			out = append(out, ai.SourceLine{Tool: "web_search", ID: d[0], Text: d[1]})
		}
	}
	// Nothing to say when a capability has one source: the model has no choice
	// to make, the tool has no `source` parameter (companionToolDefs omits a
	// single-element enum), and a line explaining a decision nobody can take is
	// tokens spent on every turn of every conversation.
	if len(out) < 2 {
		return nil
	}
	return out
}
