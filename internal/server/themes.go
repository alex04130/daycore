package server

import (
	"context"
	"net/http"
	"strings"

	"daycore/internal/domain"
)

// Which token space a theme request is judged against.
//
// # The fallback family: a manifest for the frontend that never handshakes
//
// The current frontend does not handshake yet, and third-party ones may never
// bother. Both must keep working, so a request that names no build is judged
// against a family the backend carries itself — the thirteen tokens the design
// system has always had, declared as colours.
//
// ⚠️ This is the spec's "改造期间现役前端行为不变：后端内置一份它的 manifest
// 作兜底", and it is what makes the whole migration a no-op for anybody who has
// not changed: same tokens, same values accepted, same rejections.
//
// It is NOT stored. A seeded row would be a family an operator could rename,
// pin or delete out from under a frontend that has no way to re-declare it.
//
// # How a request says which family it is
//
//	X-Frontend-Build: <buildHash>   → the build's CURRENT family, which is the
//	                                  operator's assignment when they moved it
//	absent                          → the fallback family
//
// A header rather than the session, because one session legitimately talks to
// two families at once — 桌面用琉璃、手机用汀 is the ordinary case, and a
// per-session family would make the second device retheme the first.
//
// ⚠️ An UNKNOWN build hash falls back rather than failing. A build that was
// deleted from the console, or one that has not handshaken yet, must still be
// able to read themes: refusing would turn "the operator tidied up" into "the
// app is broken".
const fallbackFamilyID = "default"

// fallbackFamily is the built-in manifest. Built once; the token list is a
// constant.
func fallbackFamily() domain.FrontendFamily {
	tokens := make([]domain.TokenSpec, 0, len(themeVarWhitelist))
	for name, desc := range themeVarWhitelist {
		tokens = append(tokens, domain.TokenSpec{Name: name, Kind: "color", Description: desc})
	}
	return domain.FrontendFamily{ID: fallbackFamilyID, DisplayName: "内置", Tokens: tokens}
}

// familyFor resolves the token space for one request.
func (s *Server) familyFor(r *http.Request) domain.FrontendFamily {
	hash := r.Header.Get(frontendBuildHeader)
	if hash == "" || s.store == nil {
		return fallbackFamily()
	}
	ctx := r.Context()
	b, err := s.store.Frontends().GetBuild(ctx, hash)
	if err != nil || b == nil {
		return fallbackFamily()
	}
	fam, err := s.store.Frontends().GetFamily(ctx, b.FamilyID)
	if err != nil || fam == nil {
		return fallbackFamily()
	}
	// ⚠️ A family with no tokens would refuse every theme write with "unknown
	// variable". That happens to a family created by a handshake that declared
	// none — legal, and a frontend in that state has not told us what it can
	// theme. Falling back is the honest answer: the built-in space is the one
	// thing we know something can render.
	if len(fam.Tokens) == 0 {
		return fallbackFamily()
	}
	return *fam
}

// frontendBuildHeader is how a request names its build.
//
// ⚠️ It is NOT a credential and must never become one. It selects a token space
// and nothing else: a caller that lies about it gets somebody else's list of
// variable names, which is public information (it is in every manifest and every
// stylesheet). Anything that started keying authority off it would be keying
// authority off an unauthenticated header.
const frontendBuildHeader = "X-Frontend-Build"

// validateThemeVariables checks every entry against the request's token space
// and each token's kind.
//
// ⚠️ Every value passes internal/theme's character floor as well as its kind's
// pattern, and the floor is the part the injection guarantee rests on. A token
// the family does not declare is REFUSED rather than ignored: accepting it would
// store a variable nothing validated and no build asked for.
// It RETURNS the normalised map, and the caller must store that rather than
// what arrived.
//
// ⚠️ Validation trims before matching a pattern, so `" #fff "` passes — and a
// caller that then stored the raw string would put whitespace into a CSS
// declaration. Returning the cleaned map makes "validated" and "what is stored"
// the same object instead of two that agree by convention. Caught by
// TestSanitizeThemeVariables, which had been asserting exactly this.
func (s *Server) validateThemeVariables(fam domain.FrontendFamily, vars map[string]string) (map[string]string, error) {
	if len(vars) == 0 {
		return nil, errThemeEmpty
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		tok, ok := fam.TokenByName(k)
		if !ok {
			return nil, unknownThemeVar(k)
		}
		v = strings.TrimSpace(v)
		if err := s.themeKinds.Validate(tok.Kind, v); err != nil {
			return nil, themeValueError(k, err)
		}
		out[k] = v
	}
	return out, nil
}

// sanitizeThemeVariables keeps only declared tokens with valid values,
// reporting what it dropped.
//
// Used on AI output, which must degrade rather than hard-fail: a model that
// invented one variable should cost that variable, not the whole theme. The
// dropped list is what tells an operator the prompt needs work.
func (s *Server) sanitizeThemeVariables(fam domain.FrontendFamily, vars map[string]string) (map[string]string, []string) {
	out := map[string]string{}
	var dropped []string
	for k, v := range vars {
		tok, ok := fam.TokenByName(k)
		if !ok {
			dropped = append(dropped, k)
			continue
		}
		v = strings.TrimSpace(v)
		if err := s.themeKinds.Validate(tok.Kind, v); err != nil {
			dropped = append(dropped, k)
			continue
		}
		out[k] = v
	}
	return out, dropped
}

// familyForSession is the family a background job should use for a session.
//
// There is no request to read a header from, so it is the fallback — and that
// is correct rather than a limitation: a job generating a theme has no device
// in front of it, and the built-in space is the one every build can render.
func (s *Server) familyForSession(_ context.Context, _ string) domain.FrontendFamily {
	return fallbackFamily()
}

// The three refusals theme validation produces, as errors rather than strings
// built at the call site — the handler turns them into one 400 shape, and the
// AI path needs to tell "not a token" from "not a valid value" apart.
var errThemeEmpty = errThemeEmptyVars{}

type errThemeEmptyVars struct{}

func (errThemeEmptyVars) Error() string { return "variables must not be empty" }

type errUnknownThemeVar struct{ name string }

func (e errUnknownThemeVar) Error() string {
	return "unknown variable " + e.name + " — this frontend's manifest does not declare it"
}

func unknownThemeVar(name string) error { return errUnknownThemeVar{name} }

type errThemeValue struct {
	name string
	err  error
}

func (e errThemeValue) Error() string { return e.name + ": " + e.err.Error() }
func (e errThemeValue) Unwrap() error { return e.err }

func themeValueError(name string, err error) error { return errThemeValue{name, err} }
