package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daycore/internal/domain"
)

// ruleInput is the wire shape for creating a rule — the same JSON the companion
// emits inside <rule_update> and the extract-schedule endpoint returns.
type ruleInput struct {
	Title       string  `json:"title"`
	Type        string  `json:"type"`
	Time        *string `json:"time"`
	DurationMin *int    `json:"duration_min"`
	Timezone    string  `json:"timezone"`
	TimeMode    string  `json:"time_mode"`
	Kind        string  `json:"kind"`
	Date        *string `json:"date"`
	Freq        string  `json:"freq"`
	Interval    int     `json:"interval"`
	ByWeekday   []int   `json:"by_weekday"`
	StartDate   string  `json:"start_date"`
	Until       *string `json:"until"`
	Active      *bool   `json:"active"`
	Source      string  `json:"source"`
	Note        *string `json:"note"`
}

var validFreqs = map[string]bool{
	domain.FreqDaily: true, domain.FreqWeekly: true,
	domain.FreqMonthly: true, domain.FreqEveryNDays: true,
}

var validBlockTypes = map[domain.BlockType]bool{
	domain.BlockTask: true, domain.BlockAppointment: true,
	domain.BlockBreak: true, domain.BlockRelax: true, domain.BlockMeal: true,
}

var validSources = map[string]bool{
	"user": true, "chat": true, "ics": true, "image": true, "canvas": true,
}

// toRule validates and converts the wire input, applying defaults.
func (in ruleInput) toRule(sid string) (*domain.ScheduleRule, error) {
	r := &domain.ScheduleRule{
		SessionID:   sid,
		Title:       strings.TrimSpace(in.Title),
		Type:        domain.BlockType(orDefault(in.Type, string(domain.BlockTask))),
		Time:        in.Time,
		DurationMin: in.DurationMin,
		Timezone:    in.Timezone,
		TimeMode:    domain.TimeMode(orDefault(in.TimeMode, string(domain.TimeFloating))),
		Kind:        orDefault(in.Kind, domain.RuleRecurring),
		Date:        in.Date,
		Freq:        in.Freq,
		Interval:    in.Interval,
		ByWeekday:   in.ByWeekday,
		StartDate:   in.StartDate,
		Until:       in.Until,
		Active:      true,
		Source:      orDefault(in.Source, "user"),
		Note:        in.Note,
	}
	if in.Active != nil {
		r.Active = *in.Active
	}
	if r.Interval < 1 {
		r.Interval = 1
	}
	if err := validateRule(r); err != nil {
		return nil, err
	}
	return r, nil
}

func validateRule(r *domain.ScheduleRule) error {
	if r.Title == "" {
		return fmt.Errorf("title is required")
	}
	if !validBlockTypes[r.Type] {
		return fmt.Errorf("invalid type %q", r.Type)
	}
	if r.TimeMode != domain.TimeFloating && r.TimeMode != domain.TimeFixed && r.TimeMode != domain.TimeLocal {
		return fmt.Errorf("invalid time_mode %q", r.TimeMode)
	}
	if r.Time != nil && *r.Time != "" {
		if _, err := time.Parse("15:04", *r.Time); err != nil {
			return fmt.Errorf("invalid time %q (want HH:MM)", *r.Time)
		}
	}
	if !validSources[r.Source] {
		return fmt.Errorf("invalid source %q", r.Source)
	}
	// The EVENT's timezone — whose wall clocks this rule's time belongs to,
	// which is a different question from where the user is right now (that is
	// SessionPrefs.Timezone). An ICS import fills it from the calendar; the
	// user can correct it from the materials page, and this is what stops that
	// edit from storing a zone nothing can load.
	//
	// Empty is legal and means "floating": the time is whatever the reader's own
	// zone says, which is the right default for "gym at 7" and the wrong one for
	// a lecture in another country.
	if r.Timezone != "" && !validTimezone(r.Timezone) {
		return fmt.Errorf("invalid timezone %q (want an IANA name like Asia/Shanghai)", r.Timezone)
	}
	switch r.Kind {
	case domain.RuleOnce:
		if r.Date == nil || !isDate(*r.Date) {
			return fmt.Errorf("kind=once requires date (YYYY-MM-DD)")
		}
	case domain.RuleRecurring:
		if !validFreqs[r.Freq] {
			return fmt.Errorf("invalid freq %q", r.Freq)
		}
		// Anchored frequencies need a start date; default it to today so chat
		// and imports don't have to spell it out.
		if r.StartDate == "" {
			r.StartDate = time.Now().Format("2006-01-02")
		}
		if !isDate(r.StartDate) {
			return fmt.Errorf("invalid start_date %q", r.StartDate)
		}
		if r.Until != nil && *r.Until != "" && !isDate(*r.Until) {
			return fmt.Errorf("invalid until %q", *r.Until)
		}
		for _, wd := range r.ByWeekday {
			if wd < 0 || wd > 6 {
				return fmt.Errorf("by_weekday values must be 0..6")
			}
		}
	default:
		return fmt.Errorf("invalid kind %q", r.Kind)
	}
	return nil
}

func isDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// applyRuleUpdate returns a copy of r with the partial update applied, mirroring
// the store's Update field-merge semantics. It lets callers validate the merged
// result BEFORE persisting, so an inconsistent update never corrupts the row.
func applyRuleUpdate(r domain.ScheduleRule, upd domain.ScheduleRuleUpdate) domain.ScheduleRule {
	if upd.Title != nil {
		r.Title = *upd.Title
	}
	if upd.Type != nil {
		r.Type = *upd.Type
	}
	if upd.HasTime {
		if upd.Time == nil || *upd.Time == "" {
			r.Time = nil
		} else {
			r.Time = upd.Time
		}
	}
	if upd.DurationMin != nil {
		r.DurationMin = upd.DurationMin
	}
	if upd.Timezone != nil {
		r.Timezone = *upd.Timezone
	}
	if upd.TimeMode != nil {
		r.TimeMode = *upd.TimeMode
	}
	if upd.Kind != nil {
		r.Kind = *upd.Kind
	}
	if upd.Date != nil {
		r.Date = upd.Date
	}
	if upd.Freq != nil {
		r.Freq = *upd.Freq
	}
	if upd.Interval != nil {
		n := *upd.Interval
		if n < 1 {
			n = 1
		}
		r.Interval = n
	}
	if upd.ByWeekday != nil {
		r.ByWeekday = *upd.ByWeekday
	}
	if upd.StartDate != nil {
		r.StartDate = *upd.StartDate
	}
	if upd.HasUntil {
		if upd.Until == nil || *upd.Until == "" {
			r.Until = nil
		} else {
			r.Until = upd.Until
		}
	}
	if upd.Active != nil {
		r.Active = *upd.Active
	}
	if upd.Note != nil {
		r.Note = upd.Note
	}
	return r
}

// ruleUpdateFromRaw builds a partial update from raw JSON, honoring nulls.
func ruleUpdateFromRaw(raw map[string]json.RawMessage) (domain.ScheduleRuleUpdate, error) {
	var upd domain.ScheduleRuleUpdate
	str := func(key string) (*string, bool, error) {
		v, ok := raw[key]
		if !ok {
			return nil, false, nil
		}
		if string(v) == "null" {
			return nil, true, nil
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, false, fmt.Errorf("%s must be a string", key)
		}
		return &s, true, nil
	}

	if v, ok, err := str("title"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Title = v
	}
	if v, ok, err := str("type"); err != nil {
		return upd, err
	} else if ok && v != nil {
		t := domain.BlockType(*v)
		upd.Type = &t
	}
	if v, ok, err := str("time"); err != nil {
		return upd, err
	} else if ok {
		upd.HasTime = true
		upd.Time = v
	}
	if v, ok := raw["duration_min"]; ok && string(v) != "null" {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return upd, fmt.Errorf("duration_min must be an integer")
		}
		upd.DurationMin = &n
	}
	if v, ok, err := str("timezone"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Timezone = v
	}
	if v, ok, err := str("time_mode"); err != nil {
		return upd, err
	} else if ok && v != nil {
		m := domain.TimeMode(*v)
		upd.TimeMode = &m
	}
	if v, ok, err := str("kind"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Kind = v
	}
	if v, ok, err := str("date"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Date = v
	}
	if v, ok, err := str("freq"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Freq = v
	}
	if v, ok := raw["interval"]; ok && string(v) != "null" {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return upd, fmt.Errorf("interval must be an integer")
		}
		upd.Interval = &n
	}
	if v, ok := raw["by_weekday"]; ok && string(v) != "null" {
		var wd []int
		if err := json.Unmarshal(v, &wd); err != nil {
			return upd, fmt.Errorf("by_weekday must be an integer array")
		}
		upd.ByWeekday = &wd
	}
	if v, ok, err := str("start_date"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.StartDate = v
	}
	if v, ok, err := str("until"); err != nil {
		return upd, err
	} else if ok {
		upd.HasUntil = true
		upd.Until = v
	}
	if v, ok := raw["active"]; ok && string(v) != "null" {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return upd, fmt.Errorf("active must be a boolean")
		}
		upd.Active = &b
	}
	if v, ok, err := str("note"); err != nil {
		return upd, err
	} else if ok && v != nil {
		upd.Note = v
	}
	return upd, nil
}
