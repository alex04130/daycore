package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// Filling in the tokens a family's stored themes are missing.
//
// # Why anything is missing in the first place
//
// A family's token space is the UNION of what its builds declare, so shipping a
// build that themes one more thing widens it — and every theme already stored
// against that family now lacks that one value. The frontend renders it with
// whatever its stylesheet defaults to, which is usually "the thing the user
// themed is suddenly not themed".
//
// # ⚠️ It costs one model call per theme, so a person asks for it
//
// A family with two thousand themes is two thousand calls, and the widening
// that caused it is a routine deploy. A backend that started spending here on
// its own would be spending because somebody shipped a frontend. So the console
// reports the price, an operator presses the button, and the request is stored
// on the family rather than kept in a goroutine — the work outlives any one
// process, and a restart halfway through must resume rather than leave half the
// themes filled with nothing recording that.
//
// # ⚠️ Why job_runs and not a loop
//
//	exclusive    (session, job, run_key) is unique and the row is written BEFORE
//	             the work, so two instances cannot both pay for the same theme.
//	resumable    a claim that dies mid-call is taken over after JobStaleAfter.
//	bounded      a theme that fails every time stops after JobMaxAttempts
//	             instead of being retried on every sweep forever — which, for a
//	             job that costs money, is the difference between a broken
//	             integration and a bill.
//
// The run key carries the family's updated_at, so widening the space again
// makes a NEW occurrence rather than colliding with the finished one. It does
// not re-arm the request: that is another decision, with another price.
const (
	// ThemeBackfillEvery is how often the leader looks for requested backfills.
	//
	// A minute rather than a tick: it is an operator-triggered job, and the
	// operator is sitting in front of the console watching a number. Anything
	// slower reads as "the button did nothing".
	ThemeBackfillEvery = time.Minute
	// ThemeBackfillPerSweep bounds one sweep's spend.
	//
	// ⚠️ A ceiling on MONEY, not on rows. Without it, pressing the button on a
	// family with two thousand themes means two thousand model calls back to
	// back, saturating the provider's rate limit and starving every interactive
	// request behind it. The sweep runs again a minute later; a large family
	// takes hours, which is the right speed for something nobody is waiting on
	// synchronously.
	ThemeBackfillPerSweep = 20
	// themeBackfillPage is the keyset page size for scanning a family.
	themeBackfillPage = 100
	// ThemeBackfillDryPasses is how many passes in a row may fill NOTHING while
	// work remains before the request is called off.
	//
	// ⚠️ Without this, a family whose themes have all used up their attempts
	// stays armed FOREVER: the sweep rescans it every minute, the console says
	// "补算进行中" forever, and nothing anywhere says it is stuck. Two ways to
	// reach that state and both are ordinary — a model that refuses this prompt,
	// or a token whose kind nothing the model writes can satisfy.
	//
	// Ten minutes rather than one, so a provider blip is ridden out rather than
	// turned into a cancelled job. A real outage means the operator presses the
	// button again, which is a decision they should be making anyway: it costs
	// money, and a backend that silently kept retrying for a day would be
	// spending on their behalf without them.
	ThemeBackfillDryPasses = 10
)

// StartThemeBackfill runs the requested backfills on the leader.
func (s *Server) StartThemeBackfill() {
	s.everyTick("theme backfill", ThemeBackfillEvery, func(parent context.Context) {
		if !s.LeadsWorker() {
			return
		}
		ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
		defer cancel()
		s.sweepThemeBackfills(ctx)
	})
}

func (s *Server) sweepThemeBackfills(ctx context.Context) {
	if s.store == nil {
		return
	}
	fams, err := s.store.Frontends().ListFamilies(ctx)
	if err != nil {
		s.log.Warn("theme backfill: could not list families", "err", err)
		return
	}
	budget := ThemeBackfillPerSweep
	for _, fam := range fams {
		if budget <= 0 {
			return
		}
		if fam.BackfillRequestedAt == nil {
			continue
		}
		spent, completed, remaining, err := s.backfillFamily(ctx, fam, budget)
		budget -= spent
		if err != nil {
			s.log.Warn("theme backfill", "family", fam.ID, "err", err)
			continue
		}
		if remaining > 0 {
			// ⚠️ PROGRESS is completions, not spend. The budget counts money —
			// including calls that fail — so "spent > 0" is true on every pass of
			// a permanently broken family, and reading it here would mean the
			// stuck check never fires. Two numbers, two questions, and using the
			// wrong one is silent.
			if completed > 0 {
				delete(s.backfillDry, fam.ID)
				s.log.Info("theme backfill in progress",
					"family", fam.ID, "filled", completed, "calls", spent, "left", remaining)
				continue
			}
			// A pass that COMPLETED nothing while work remained. In memory rather
			// than on the row: the leader is one process, and a new leader
			// re-counting from zero is the right behaviour — it gets its own ten
			// minutes to find out whether the thing is actually stuck.
			s.backfillDry[fam.ID]++
			if s.backfillDry[fam.ID] < ThemeBackfillDryPasses {
				continue
			}
			s.log.Warn("theme backfill is stuck and has been called off",
				"family", fam.ID, "themes_still_missing_tokens", remaining,
				"why", "every remaining theme failed or has used up its attempts; press it again to retry")
		}
		delete(s.backfillDry, fam.ID)
		s.clearBackfillIfUnchanged(ctx, fam)
	}
}

// clearBackfillIfUnchanged disarms a family whose backfill this pass finished.
//
// ⚠️ Only the request WE were working on. There is no compare-and-set on
// families, so this re-read plus comparison is the whole defence: an operator
// who pressed the button again while the sweep was running — because the token
// space widened again — has a newer timestamp, and clearing it would silently
// throw away a request they just made and are watching a screen for.
//
// A separate method rather than four lines inline because that comparison is
// the entire invariant, and an invariant that lives only inside a loop body is
// one nothing can assert on directly.
func (s *Server) clearBackfillIfUnchanged(ctx context.Context, fam domain.FrontendFamily) {
	cur, err := s.store.Frontends().GetFamily(ctx, fam.ID)
	if err != nil {
		return
	}
	if cur.BackfillRequestedAt == nil {
		return
	}
	// ⚠️ Compared at MILLISECOND granularity, because that is the precision the
	// stores keep (epoch millis on all four). Comparing with Equal works only
	// while both sides came out of a read; the moment a caller passes a time it
	// made itself, Equal is false against the truncated stored copy and the
	// request is never cleared — a backfill that rescans every minute forever,
	// with nothing logged.
	if fam.BackfillRequestedAt == nil ||
		cur.BackfillRequestedAt.UnixMilli() != fam.BackfillRequestedAt.UnixMilli() {
		s.log.Info("theme backfill: a newer request arrived while this pass was running; leaving it armed",
			"family", fam.ID)
		return
	}
	cur.BackfillRequestedAt = nil
	if err := s.store.Frontends().UpsertFamily(ctx, *cur); err != nil {
		s.log.Warn("theme backfill: could not clear the request", "family", fam.ID, "err", err)
		return
	}
	s.log.Info("theme backfill finished", "family", fam.ID)
}

// backfillFamily works through one family and reports three different numbers.
//
//	spent       model calls made — MONEY, the thing budget bounds. Counts calls
//	            that failed, because those cost the same.
//	completed   themes that now have every token — PROGRESS. A pass with spend
//	            but no completions is getting nowhere, which is what the stuck
//	            check reads.
//	remaining   themes still missing tokens — the operator's countdown.
//
// ⚠️ It finishes the scan even after the budget is spent, so `remaining` is the
// real countdown rather than "at least". That costs one query per hundred themes
// and buys the operator a number that goes down — the difference between a
// progress bar and a spinner, on a job that can run for hours.
func (s *Server) backfillFamily(ctx context.Context, fam domain.FrontendFamily, budget int) (spent, completed, remaining int, err error) {
	after := ""
	for {
		page, err := s.store.Themes().ScanFamily(ctx, fam.ID, after, themeBackfillPage)
		if err != nil {
			return spent, completed, remaining, err
		}
		if len(page) == 0 {
			return spent, completed, remaining, nil
		}
		for _, th := range page {
			after = th.ID
			missing := fam.MissingTokens(th.Variables)
			if len(missing) == 0 {
				continue
			}
			remaining++
			if spent >= budget {
				continue
			}
			// ⚠️ paid and complete are SEPARATE, and conflating them defeated the
			// budget entirely: a model that fails, or that fills only some of the
			// missing tokens, leaves the theme incomplete — so counting only
			// completions kept `spent` at zero and let every theme in the family
			// get a call in one sweep. Precisely the case where a ceiling on
			// money matters most: a misbehaving model burning through two
			// thousand themes back to back.
			paid, complete := s.fillOneTheme(ctx, fam, th, missing)
			if paid {
				spent++
			}
			if complete {
				completed++
				remaining--
			}
		}
		if ctx.Err() != nil {
			return spent, completed, remaining, ctx.Err()
		}
	}
}

// fillOneTheme claims the occurrence, calls the model, and stores what survives
// validation.
//
//	paid       this theme's occurrence was claimed, so the call was made and the
//	           money is gone. FALSE only when somebody else holds the claim or it
//	           has used up its attempts — the two cases where nothing is spent.
//	complete   every missing token actually landed. A partial fill is a real
//	           outcome (one token whose kind the model could not satisfy) and
//	           reporting it as complete would let the sweep declare a family done
//	           while a theme is still broken.
//
// ⚠️ They are separate because the budget counts MONEY and the countdown counts
// WORK. One value for both is how the ceiling stopped applying to exactly the
// calls that fail.
func (s *Server) fillOneTheme(ctx context.Context, fam domain.FrontendFamily, th domain.CustomTheme, missing []domain.TokenSpec) (paid, complete bool) {
	run := &domain.JobRun{
		SessionID: th.SessionID,
		Job:       domain.JobThemeBackfill,
		// The occurrence is "this theme, against this version of the token
		// space" — so widening the family is a new occurrence rather than a
		// collision with the finished one.
		//
		// ⚠️ Keyed on the SPACE, not on fam.UpdatedAt. The handshake upserts the
		// family on every connection, so updated_at moves whenever any frontend
		// loads a page — and a run key built from it rotated with it, minting a
		// fresh occurrence each time and resetting the attempts counter. A theme
		// that could never be filled would have been paid for forever, with
		// every job_run showing one clean attempt.
		RunKey: th.ID + "@" + fam.TokenSpaceHash(),
	}
	ok, err := s.store.JobRuns().Claim(ctx, run)
	if err != nil || !ok {
		return false, false
	}
	filled, err := s.generateMissing(ctx, fam, th, missing)
	if err != nil {
		_ = s.store.JobRuns().Finish(ctx, run.ID, domain.JobFailed, err.Error(), time.Now())
		return true, false
	}
	// ⚠️ MERGED into a FRESH read, never replaced, and never merged into the
	// snapshot this sweep started with.
	//
	// Two separate reasons, both about the same worst outcome — silently
	// destroying a palette somebody chose, for which there is no undo:
	//
	//  1. The model was told not to touch what is there, but "was told" is not a
	//     guarantee, so only the MISSING tokens are taken from its answer.
	//  2. The model call takes seconds, and Update writes the whole variables
	//     map. Merging into the copy read before the call would revert any edit
	//     the user made in that window. Re-reading narrows it from seconds to
	//     the microseconds between this read and the write — the same exposure
	//     every other read-modify-write here carries, rather than a new and much
	//     wider one.
	//
	// A theme deleted during the call is not an error: nothing to fill.
	fresh, err := s.store.Themes().Get(ctx, th.SessionID, th.ID)
	if err != nil {
		_ = s.store.JobRuns().Finish(ctx, run.ID, domain.JobDone, "", time.Now())
		return true, false
	}
	vars := map[string]string{}
	for k, v := range fresh.Variables {
		vars[k] = v
	}
	added := 0
	for _, t := range missing {
		v, ok := filled[t.Name]
		if !ok {
			continue
		}
		// The user may have filled it themselves while the model was thinking.
		// Theirs wins — they chose it, the model guessed it.
		if _, theirs := fresh.Variables[t.Name]; theirs {
			added++
			continue
		}
		if err := s.themeKinds.Validate(t.Kind, v); err != nil {
			continue
		}
		vars[t.Name] = v
		added++
	}
	if added == 0 {
		_ = s.store.JobRuns().Finish(ctx, run.ID, domain.JobFailed,
			"the model returned nothing that passed validation", time.Now())
		return true, false
	}
	if _, err := s.store.Themes().Update(ctx, th.SessionID, th.ID,
		domain.CustomThemeUpdate{Variables: &vars}); err != nil {
		_ = s.store.JobRuns().Finish(ctx, run.ID, domain.JobFailed, err.Error(), time.Now())
		return true, false
	}
	// ⚠️ A partial fill closes the occurrence as DONE, not failed, because the
	// work that succeeded must not be retried and paid for again. The theme is
	// still incomplete, which `complete=false` reports — and the family's
	// countdown is what keeps it visible.
	_ = s.store.JobRuns().Finish(ctx, run.ID, domain.JobDone, "", time.Now())
	return true, added == len(missing)
}

func (s *Server) generateMissing(ctx context.Context, fam domain.FrontendFamily, th domain.CustomTheme, missing []domain.TokenSpec) (map[string]string, error) {
	if s.prompts == nil || s.catalog == nil {
		return nil, errors.New("no model configured")
	}
	rules := ""
	if fam.RulesAccepted {
		rules = fam.Rules
	}
	// ⚠️ The deployment default locale, not a user's. Nobody is watching this
	// call, and the values it produces are colours and lengths — the locale only
	// picks which copy of the prompt is used, and using the requester's would
	// mean the same theme was filled differently depending on who happened to
	// press the button.
	sys, err := s.prompts.Render(ctx, ai.PromptThemeBackfill, s.defaultLocales.Primary, ai.ThemeBackfillData{
		ThemeName:        th.Name,
		CurrentVariables: marshalCompact(th.Variables),
		Dark:             th.Dark,
		MissingVars:      s.tokenListMarkdown(missing),
		FamilyRules:      rules,
	})
	if err != nil {
		return nil, err
	}
	provider := s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleSystem, Content: sys}, {Role: ai.RoleUser, Content: th.Name}},
		Temperature: 0.4, MaxTokens: 1200, JSONMode: true,
	})
	// ⚠️ Its own endpoint, and NOT billed to the account — the operator asked for
	// this, not the person whose theme it is. See logAICallNotBilledToTheAccount.
	s.logAICallNotBilledToTheAccount(ctx, th.SessionID, epThemeBackfill, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		return nil, err
	}
	result, ok := extractJSONObject(resp.Content)
	if !ok {
		return nil, errors.New("the model did not return a JSON object")
	}
	raw, _ := result["variables"].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			out[k] = sv
		}
	}
	if len(out) == 0 {
		// Some providers answer with the bare object when asked for one field.
		b, _ := json.Marshal(result)
		var flat map[string]string
		if json.Unmarshal(b, &flat) == nil {
			out = flat
		}
	}
	return out, nil
}

// themeBackfillCount reports how many themes in a family are missing tokens —
// the price, before anybody presses anything.
//
// ⚠️ Bounded. A count that walked a two-thousand-theme family on every console
// load would make the screen slow in exactly the deployment where the number
// matters most, so it stops at max and says it stopped. "至少 500 套" is an
// honest answer; a number that took eight seconds to be exact is not a better
// one for a decision that is "is this a lot".
func (s *Server) themeBackfillCount(ctx context.Context, fam domain.FrontendFamily, max int) (count int, capped bool, err error) {
	after := ""
	for {
		page, err := s.store.Themes().ScanFamily(ctx, fam.ID, after, themeBackfillPage)
		if err != nil {
			return count, false, err
		}
		if len(page) == 0 {
			return count, false, nil
		}
		for _, th := range page {
			after = th.ID
			if len(fam.MissingTokens(th.Variables)) > 0 {
				count++
				if count >= max {
					return count, true, nil
				}
			}
		}
	}
}
