package setup

import (
	"fmt"
	"strconv"
	"strings"

	"daycore/internal/i18n"
)

// Terminal presentation. Kept apart from the flow so that adding a question
// never means touching escape codes, and so the one place that decides whether
// colour is emitted is one place.

const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cGreen = "\033[32m"
	cCyan  = "\033[36m"
	cRed   = "\033[31m"
	cAmber = "\033[33m"
)

// c returns an escape sequence, or "" when the output is not a terminal.
func (s *Session) c(code string) string {
	if !s.Color {
		return ""
	}
	return code
}

func (s *Session) printf(format string, args ...any) {
	fmt.Fprintf(s.Out, format, args...)
}

// Heading starts a section.
func (s *Session) Heading(key string, args ...any) {
	s.printf("\n  %s%s%s\n\n", s.c(cBold), i18n.Tf(key, s.Locale, args...), s.c(cReset))
}

// Done reports something that happened.
func (s *Session) Done(key string, args ...any) {
	s.printf("  %s✓%s %s\n", s.c(cGreen), s.c(cReset), i18n.Tf(key, s.Locale, args...))
}

// Hint is context the reader does not have to act on.
func (s *Session) Hint(key string, args ...any) {
	s.printf("  %s%s%s\n", s.c(cDim), i18n.Tf(key, s.Locale, args...), s.c(cReset))
}

// Warn is something the operator has to know but that does not stop the flow.
//
// Distinct from Done and from a returned error on purpose: this installer's
// worst historical failure was reporting success over a directory that could
// not start, so "it worked but read this" needs a shape of its own that is not
// a green tick.
func (s *Session) Warn(key string, args ...any) {
	s.printf("  %s!%s %s\n", s.c(cAmber), s.c(cReset), i18n.Tf(key, s.Locale, args...))
}

// Fail is a hard problem, printed before the command exits non-zero.
func (s *Session) Fail(key string, args ...any) {
	s.printf("  %s✗%s %s\n", s.c(cRed), s.c(cReset), i18n.Tf(key, s.Locale, args...))
}

// Plain writes text that is not a message key — a path, a DSN, a generated
// token. Anything that would be the same in every language belongs here rather
// than in the catalog, because a key whose translation is "%s" is a key that
// only makes the pack longer.
func (s *Session) Plain(text string) {
	s.printf("  %s\n", text)
}

// Rule draws the separator between the questions and the summary.
func (s *Session) Rule() {
	s.printf("  %s%s%s\n", s.c(cDim), strings.Repeat("─", 46), s.c(cReset))
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return -1
	}
	return n
}
