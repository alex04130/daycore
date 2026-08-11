package server

import (
	"context"
	"errors"
	"sort"
	"strings"

	"daycore/internal/domain"
	"daycore/internal/theme"
)

// ReloadThemeKinds pulls the database layer of the kind registry into memory.
//
// Same shape and the same reason as ReloadLocaleOverrides: `theme.Registry`
// holds a `db` map it never receives on its own, `domain.ThemeKindRepository`
// is implemented in both stores and covered by the conformance suite, and
// nothing connects them. Every claim about approving a kind from the console —
// or about an approval reaching the other instances — goes through here, so the
// wiring IS the claim.
//
// The fold lives in this package rather than in `internal/theme` on purpose:
// the registry is deliberately storage-free (it is the reason the whole kind
// system is testable without a database), so the package that owns the store
// owns the loading.
//
// ⚠️ Only APPROVED rows are merged. An unapproved row exists to be looked at
// and nothing else — that is the entire content of the third tier's gate.
//
// Call it at startup and again after any console edit. It replaces the layer
// wholesale, which is why List must return everything: a partial read would
// silently revoke kinds, and a revoked kind makes every theme declaring it fail
// with "unknown kind".
//
// # ⬜ Known gap: it reloads THIS instance
//
// An approval made on instance A does not reach instance B until B restarts or
// somebody edits a kind there too. Deliberately shared with the message
// catalogue, which has the same shape and the same gap — this is not the place
// to invent a cache-invalidation bus for one table.
//
// It is bounded rather than open-ended: the DATABASE is the source of truth, so
// the instances disagree about what validates until the next reload and never
// about what is stored. The visible symptom is a theme write that succeeds on
// one node and is refused on another for a few minutes, which is annoying and
// not corrupting.
//
// When it does get fixed, fix it for both — a mechanism that carried theme
// kinds and not translations would be the more confusing outcome.
func (s *Server) ReloadThemeKinds(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.ThemeKinds().List(ctx)
	if err != nil {
		return err
	}
	approved := make([]theme.Kind, 0, len(rows))
	for _, r := range rows {
		if !r.Approved {
			continue
		}
		approved = append(approved, theme.Kind{
			Name: r.Name, Pattern: r.Pattern, Description: r.Description,
		})
	}
	for _, problem := range s.themeKinds.SetDBKinds(approved) {
		// Reported, not returned: one row that stopped compiling must not stop
		// the other approved kinds from loading, and at boot it must not stop
		// the process. An operator sees it in the log and on the screen (the
		// row is still there, still approved, and now visibly broken).
		s.log.Warn("theme kind could not be loaded", "err", problem)
	}
	return nil
}

// proposedKindNameOK reports whether a FRONTEND may propose this name.
//
// ⚠️ A frontend may not propose a name that already means something. Two
// reasons, and only the second is obvious:
//
//   - a combinator (`one-of[…]`) is parsed rather than stored, so a row by that
//     name could never take effect and would sit in the console forever looking
//     like a pending decision.
//   - an embedded PRIMITIVE (`color`, `length`, …) is the floor every existing
//     theme was validated against. A frontend proposing `color` and an operator
//     approving it without reading closely would widen validation for every
//     theme on the deployment, retroactively. An operator can still redefine a
//     primitive — through THEME_KINDS_DIR, a file on the server's own disk,
//     which is a deliberate act by somebody who already has the machine. What
//     is refused here is a stranger asking them to.
func proposedKindNameOK(name string) bool {
	if !theme.PlainKindName(name) {
		return false
	}
	for _, k := range theme.EmbeddedKinds() {
		if k.Name == name {
			return false
		}
	}
	return true
}

// kindView is one row as the console sees it.
type kindView struct {
	domain.ThemeKind
	// InForce says this kind is actually validating writes right now. It is not
	// the same as Approved: a row can be approved and still not load, because
	// its pattern stopped compiling or exceeded the bound.
	//
	// ⚠️ Shown separately for exactly that case. "Approved" with nothing
	// enforcing it is the state an operator most needs to be able to see, and
	// the one a single boolean hides.
	InForce bool `json:"inForce"`
	// Shadows names the layer this kind is overriding, or "" — an operator
	// approving a kind that redefines a primitive should be told so on the row,
	// not in a changelog.
	Shadows string `json:"shadows,omitempty"`
}

// recordProposedKinds stores the third tier a manifest asks for and reports
// which names are now waiting on a person.
//
// # ⚠️ Proposing is not installing
//
// A proposed row is stored UNAPPROVED, is not merged into the registry, and
// validates nothing. The whole tier is "a frontend can ASK", and the asking is
// separated from the granting because a pattern is a promise about what a token
// may hold — a promise nobody agreed to is not a promise.
//
// # ⚠️ Re-proposing DIFFERENT text revokes approval
//
// Approval means "I read this". It cannot outlive the text it was granted
// against, or the gate is one handshake away from decorative: a frontend gets
// `^[0-9]+$` approved, ships `.*` next week, and the deployment runs whatever
// arrived later with the approved flag still set.
//
// Re-proposing the SAME text does not revoke, or every page load would undo the
// operator's decision. Exactly the rule `theme.rules` already follows a few
// lines away in the handshake, and expressed the same way — by comparing the
// text rather than by storing a hash of it, because two representations of one
// fact is one more than the fact needs.
//
// # What this never does
//
// It never returns an error to the caller. A proposal that cannot be stored —
// the ceiling is reached, the name is taken by a primitive, the pattern will not
// compile — costs that one kind and is reported in the response. Refusing the
// whole handshake over it would mean a frontend that added one experimental
// token could not connect at all.
func (s *Server) recordProposedKinds(ctx context.Context, familyID string, proposed []manifestKind) map[string]bool {
	pending := map[string]bool{}
	if len(proposed) == 0 || s.store == nil {
		return pending
	}
	repo := s.store.ThemeKinds()
	pendingCount, err := repo.CountPending(ctx)
	if err != nil {
		s.log.Warn("could not count pending theme kinds", "err", err)
		return pending
	}
	for _, k := range proposed {
		name := strings.TrimSpace(k.Name)
		if !proposedKindNameOK(name) {
			// Silently skipped rather than reported: a frontend proposing
			// `color` is either confused or trying it on, and neither deserves a
			// row in an operator's queue. The refusal is in the log for the
			// confused case.
			s.log.Info("a frontend proposed a theme kind name it may not have",
				"family", familyID, "kind", name)
			continue
		}
		// ⚠️ TRUNCATED for the description, REFUSED for the pattern.
		//
		// cleanManifestText cuts a string to length, which is right for prose —
		// a description clipped at 200 characters is a description. It is wrong
		// for EXECUTABLE text: a regex cut at 512 characters is a different
		// regex, and some truncations still compile and mean something else
		// entirely. Bounding a pattern means saying no, not saying less.
		// ⚠️ The +1 is load-bearing, and the asymmetry with the line below it is
		// the point. cleanManifestText TRUNCATES, which is right for prose — a
		// description clipped at 200 characters is still a description — and
		// wrong for EXECUTABLE text: a regex cut to length is a different regex,
		// and some truncations still compile and mean something else, so an
		// operator would read and approve one pattern while another validated
		// every write. Cutting at limit+1 leaves anything legal untouched and
		// leaves anything illegal one character too long, which CompileCheck
		// then refuses. Bounding a pattern means saying no, not saying less.
		pattern := cleanManifestText(k.Pattern, theme.MaxStoredPattern+1)
		desc := cleanManifestText(k.Description, domain.MaxKindDescription)
		if pattern == "" {
			continue
		}
		// Compiled here so a pattern that could never work never reaches an
		// operator's screen. The registry anchors it; this is the same anchor,
		// because a pattern that compiles bare and not anchored would be
		// approved and then fail to load.
		if _, err := theme.CompileCheck(pattern); err != nil {
			s.log.Info("a frontend proposed a theme kind whose pattern will not compile",
				"family", familyID, "kind", name, "err", err)
			continue
		}

		existing, err := repo.Get(ctx, name)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			if pendingCount >= domain.MaxProposedKinds {
				// ⚠️ The handshake is unauthenticated, so proposals arrive from
				// anything that can reach the server. Same answer as the family
				// ceiling: a bound plus a person, not a credential a
				// first-contact frontend could not have.
				s.log.Warn("refusing a theme kind proposal: the deployment is at its pending limit",
					"family", familyID, "kind", name, "limit", domain.MaxProposedKinds)
				continue
			}
			if err := repo.Upsert(ctx, domain.ThemeKind{
				Name: name, Pattern: pattern, Description: desc, ProposedBy: familyID,
			}); err != nil {
				s.log.Warn("could not record a theme kind proposal", "kind", name, "err", err)
				continue
			}
			pendingCount++
			pending[name] = true
		case err != nil:
			s.log.Warn("could not read a theme kind", "kind", name, "err", err)
		case existing.Pattern == pattern && existing.Description == desc:
			// Unchanged. Do not write — a page load must not move updated_at,
			// and must not disturb an approval.
			if !existing.Approved {
				pending[name] = true
			}
		default:
			// Changed. The approval was granted against text that no longer
			// exists, so it goes.
			was := existing.Approved
			existing.Pattern, existing.Description, existing.Approved = pattern, desc, false
			if err := repo.Upsert(ctx, *existing); err != nil {
				s.log.Warn("could not update a theme kind proposal", "kind", name, "err", err)
				continue
			}
			if was {
				s.log.Warn("a theme kind's approval was revoked because the frontend changed its pattern",
					"kind", name, "family", familyID)
				// ⚠️ The registry still holds the OLD approved pattern until a
				// reload, and the reload is what makes the revocation real.
				if err := s.ReloadThemeKinds(ctx); err != nil {
					s.log.Warn("could not reload theme kinds after a revocation", "err", err)
				}
			}
			pending[name] = true
		}
	}
	return pending
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
