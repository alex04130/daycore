package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/resources"
)

// Known prompt keys (one editable template each, per locale).
const (
	PromptDayPlanText          = "day_plan_text"
	PromptDayPlanImage         = "day_plan_image"
	PromptMood                 = "mood"
	PromptCompanionAgent       = "companion_agent"
	PromptCompanionContext     = "companion_context"
	PromptAutoPlan             = "auto_plan"
	PromptScheduleExtractImage = "schedule_extract_image"
	PromptThemeGen             = "theme_gen"
	PromptInboxClassify        = "inbox_classify"
	PromptFoodRecognize        = "food_recognize"
	PromptTravelSuggest        = "travel_suggest"
	// Moved out of Go string literals (2026-08-03). They were switch statements
	// on strings.HasPrefix(locale, "zh") — three prompts that could not be
	// overridden from the console, could not gain a third language without a
	// release, and were exempt from the double-locale boot check that every
	// other prompt has to pass.
	PromptPersona = "persona"
	PromptBrief   = "brief"
	PromptReplan  = "replan"
)

// promptKeys is the canonical ordered list, used by List().
var promptKeys = []string{
	PromptDayPlanText, PromptDayPlanImage, PromptMood,
	PromptCompanionAgent, PromptCompanionContext,
	PromptAutoPlan, PromptScheduleExtractImage, PromptThemeGen,
	PromptInboxClassify, PromptFoodRecognize, PromptTravelSuggest,
	PromptPersona, PromptBrief, PromptReplan,
}

// PromptService renders prompt templates per locale. Defaults are embedded from
// prompts/<locale>/<key>.tmpl (every supported locale must ship every key); a
// domain.PromptRepository may override any (key, locale) pair at runtime via the
// admin API. Overrides win over the embedded default. Unknown locales fall back
// to i18n.Default.
type PromptService struct {
	repo     domain.PromptRepository
	defaults map[string]map[string]string // locale → key → template text
	mu       sync.RWMutex
}

// NewPromptService loads the embedded defaults for every supported locale.
// repo may be nil (defaults only).
func NewPromptService(repo domain.PromptRepository) (*PromptService, error) {
	s := &PromptService{repo: repo, defaults: map[string]map[string]string{}}
	for _, locale := range i18n.Embedded {
		s.defaults[locale] = map[string]string{}
		for _, key := range promptKeys {
			b, err := resources.Read("prompts/" + locale + "/" + key + ".tmpl")
			if err != nil {
				return nil, fmt.Errorf("load default prompt %q (%s): %w", key, locale, err)
			}
			s.defaults[locale][key] = string(b)
		}
	}
	return s, nil
}

// LoadDiskDefaults overlays templates found under dir onto the embedded ones:
// dir/<locale>/<key>.tmpl replaces that one default, anything absent keeps the
// embedded text. It returns how many files were applied.
//
// Embedded is the floor, the same rule the message catalog uses — and for the
// same reason. An all-or-nothing loader means `daycore install` has to extract
// every template before the server will start, so editing one prompt costs you
// the maintenance of twenty-two files: every later change to a shipped template
// silently stops reaching you. Overlaying makes a partial extraction the normal
// case.
//
// This exists because `daycore install` already writes the templates to
// `<dir>/prompts/` and puts `PROMPTS_DIR` in the generated `.env` — for a while
// nothing read it, so the installer was setting up an override the server
// ignored.
func (s *PromptService) LoadDiskDefaults(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	applied := 0
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, locale := range i18n.Embedded {
		for _, key := range promptKeys {
			path := filepath.Join(dir, locale, key+".tmpl")
			b, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return applied, fmt.Errorf("read prompt %q (%s) from %s: %w", key, locale, path, err)
			}
			if strings.TrimSpace(string(b)) == "" {
				// A truncated file is a likelier explanation than "I meant to
				// blank this prompt" — an empty system prompt is never intended.
				return applied, fmt.Errorf("prompt %q (%s) at %s is empty", key, locale, path)
			}
			if _, err := template.New(key).Parse(string(b)); err != nil {
				// Refuse at load, not at the first user request: a template that
				// cannot parse would otherwise fail one feature at runtime with no
				// hint that a file on disk is why.
				return applied, fmt.Errorf("prompt %q (%s) at %s does not parse: %w", key, locale, path, err)
			}
			s.defaults[locale][key] = string(b)
			applied++
		}
	}
	return applied, nil
}

// normLocale collapses any tag onto a shipped locale, defaulting when unknown.
func normLocale(locale string) string {
	if l := i18n.Normalize(locale); l != "" {
		return l
	}
	return i18n.Default
}

// Render resolves the active template for key+locale (override or default) and
// executes it against data.
func (s *PromptService) Render(ctx context.Context, key, locale string, data any) (string, error) {
	tmplText, err := s.active(ctx, key, normLocale(locale))
	if err != nil {
		return "", err
	}
	t, err := template.New(key).Option("missingkey=zero").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("parse prompt %q: %w", key, err)
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render prompt %q: %w", key, err)
	}
	return sb.String(), nil
}

// Get returns the active prompt text for key+locale (override if present, else default).
func (s *PromptService) Get(ctx context.Context, key, locale string) (string, bool, error) {
	if !s.known(key) {
		return "", false, domain.ErrNotFound
	}
	text, err := s.active(ctx, key, normLocale(locale))
	return text, true, err
}

// Default returns the embedded default for a key+locale.
func (s *PromptService) Default(key, locale string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.defaults[normLocale(locale)][key]
	return v, ok
}

// Set validates the template parses, then persists it as an override for locale.
func (s *PromptService) Set(ctx context.Context, key, locale, content string) error {
	if !s.known(key) {
		return domain.ErrNotFound
	}
	if !i18n.IsEmbedded(locale) {
		return fmt.Errorf("unsupported locale %q", locale)
	}
	if _, err := template.New(key).Parse(content); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	if s.repo == nil {
		return fmt.Errorf("prompt overrides are not available (no store)")
	}
	return s.repo.Set(ctx, key, locale, content)
}

// Keys returns the known prompt keys.
func (s *PromptService) Keys() []string { return append([]string(nil), promptKeys...) }

func (s *PromptService) known(key string) bool {
	_, ok := s.defaults[i18n.Default][key]
	return ok
}

func (s *PromptService) active(ctx context.Context, key, locale string) (string, error) {
	if s.repo != nil {
		if p, err := s.repo.Get(ctx, key, locale); err == nil && p != nil && strings.TrimSpace(p.Content) != "" {
			return p.Content, nil
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.defaults[locale][key]; ok {
		return v, nil
	}
	if v, ok := s.defaults[i18n.Default][key]; ok {
		return v, nil
	}
	return "", domain.ErrNotFound
}

// ─── Template data structs ──────────────────────────────────────────────────

// PlanTextData feeds prompts/<locale>/day_plan_text.tmpl.
type PlanTextData struct {
	Date, Weekday, Time, Timezone string
	TargetDate, TargetWeekday     string
	RelativeDateMap               string
}

// PlanImageData feeds prompts/<locale>/day_plan_image.tmpl and
// schedule_extract_image.tmpl.
type PlanImageData struct {
	Date, Weekday, Time, Timezone string
}

// MoodData feeds prompts/<locale>/mood.tmpl.
type MoodData struct {
	Mood        string
	MemoryFacts string // JSON [{id, fact}]; lets responses feel personal
}

// InboxClassifyData feeds prompts/<locale>/inbox_classify.tmpl — the
// quick-capture AI classifier (text → category + structured fields).
type InboxClassifyData struct {
	Text       string // the raw capture
	Categories string // markdown list of the session's enabled categories (id + name + hint)
	Date       string // today YYYY-MM-DD, base for resolving relative dates
}

// FoodRecognizeData feeds prompts/<locale>/food_recognize.tmpl (photo → diet
// entry with nutrition estimates). No variables — the image rides the vision
// request, not the template.
type FoodRecognizeData struct{}

// TravelSuggestData feeds prompts/<locale>/travel_suggest.tmpl (destination +
// dates → itinerary suggestion).
type TravelSuggestData struct {
	Destination string
	StartDate   string // YYYY-MM-DD, may be empty
	EndDate     string // YYYY-MM-DD, may be empty
	Notes       string // free-form extra requirements
	Date        string // today YYYY-MM-DD
}

// CompanionAgentData feeds prompts/<locale>/companion_agent.tmpl (L1 hard
// boundaries — pure rules, zero personality). No fields; the template is a
// static rule list.
type CompanionAgentData struct {
	// Intentionally empty — L1 is a pure rule list with no template variables.
}

// CompanionContextData feeds prompts/<locale>/companion_context.tmpl (L3 data
// block, re-rendered per turn).
type CompanionContextData struct {
	Date, Weekday, Time, Timezone string
	RelativeDateMap               string
	WeatherSummary                string // "" hides the weather line
	TodayPlan                     string // JSON
	TomorrowPlan                  string // JSON
	MemoryFacts                   string // JSON [{id, fact}]
	AssignmentsContext            string // markdown/JSON summary of upcoming assignments
	RulesContext                  string // JSON summary of schedule rules (with ids)
	MoodHistory                   string // JSON
}

// ThemeGenData feeds prompts/<locale>/theme_gen.tmpl.
type ThemeGenData struct {
	Description      string // what the user asked for
	AllowedVars      string // markdown list of themeable CSS variables + meaning
	BaseName         string // builtin base theme name ("" when none)
	BaseVariables    string // JSON of the base theme's variables ("" when none)
	CurrentName      string // edit mode: the theme being edited ("" when creating)
	CurrentVariables string // edit mode: its current variables JSON
}

// AutoPlanData feeds prompts/<locale>/auto_plan.tmpl.
type AutoPlanData struct {
	Date, Weekday, Time, Timezone string
	RelativeDateMap               string
	From, To                      string // planning range, inclusive
	Dates                         string // markdown list of every date in the range
	FixedBlocks                   string // JSON array of immovable blocks
	Assignments                   string // markdown table of upcoming assignments
	Courses                       string // markdown list of courses + grades
	KeyFacts                      string // JSON array of long-term memory facts
	Instructions                  string // optional extra user instructions
}

// The L1 hard boundary block lives in boundaries.go — a JSON file with no
// database layer, deliberately outside this service. See that file for why.
