package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daycore/internal/domain"
)

func (s *Server) toolGetWeather(ctx context.Context, sid, locale, rawArgs string) toolResult {
	var args struct {
		Location string `json:"location"`
		Days     int    `json:"days"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || strings.TrimSpace(args.Location) == "" {
		return toolFail("location is required")
	}
	if s.weather == nil {
		return toolFail("weather is not configured")
	}
	f, err := s.weather.Lookup(ctx, domain.WeatherQuery{Location: args.Location, Days: args.Days, Locale: locale})
	if err != nil {
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
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || strings.TrimSpace(args.Query) == "" {
		return toolFail("query is required")
	}
	if args.MaxResults <= 0 {
		args.MaxResults = 3
	}
	results, err := s.search.Search(ctx, args.Query, args.MaxResults)
	if err != nil {
		return toolFail("search failed: %v", err)
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "tool_web_search", Summary: args.Query,
	})
	return toolResult{OK: true, Data: map[string]any{"results": results}, Summary: args.Query}
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
