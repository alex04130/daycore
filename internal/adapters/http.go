package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The HTTP transport every `format: http` adapter is reached through.
//
// docs/specs/transport.md is the specification; this file is that document in
// Go, and it is shared by every capability for the reason the document itself
// gives: a rule written down four times drifts four ways, and the symptom is
// "the same adapter works mounted as storage and fails intermittently as
// weather", with nothing in any log that says so.

// ErrKind classifies a failure so the caller can decide without parsing text.
type ErrKind string

const (
	// KindRequest is our fault: a malformed request, an operation the adapter
	// does not implement. Never retried — retrying a programming error just
	// makes three of them.
	KindRequest ErrKind = "request"
	// KindAuth is a token problem. Never retried, and it marks the source down
	// immediately rather than after the usual streak: a wrong token does not
	// become right, and the reason is actionable.
	KindAuth ErrKind = "auth"
	// KindNotFound is the adapter saying the thing does not exist.
	KindNotFound ErrKind = "not_found"
	// KindThrottled is 429. Retried with backoff.
	KindThrottled ErrKind = "throttled"
	// KindUpstream is 5xx — the adapter itself is broken. Retried.
	KindUpstream ErrKind = "upstream"
	// KindProtocol is a response that is not what the protocol says: a redirect,
	// an unparseable body, a 200 with nothing in it. Not retried, because
	// repeating a request whose answer we cannot read produces the same
	// unreadable answer.
	KindProtocol ErrKind = "protocol"
	// KindTransport is a dial failure, a timeout, a reset connection.
	KindTransport ErrKind = "transport"
)

// Error is what every adapter call returns on failure.
//
// # Two texts, deliberately
//
// Error() is safe to show a model, put in an API response, or hand a user: it
// names the source id, the kind and the status, and nothing else. Detail()
// carries the URL and the adapter's own words and goes to the log only.
//
// The reason is concrete: an adapter's error text routinely quotes the URL it
// was called on, that URL can contain an internal hostname or a key in a query
// string, and a tool failure travels straight into the model's context and from
// there into what the assistant tells the user. A message that helps debugging
// and a message that is safe to repeat are not the same message.
type Error struct {
	Kind   ErrKind
	Source string
	Status int
	detail string
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Source, e.Kind, e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Source, e.Kind)
}

// Detail is the full text, for logs only.
func (e *Error) Detail() string { return e.detail }

// Retryable reports whether another attempt could plausibly succeed.
func (e *Error) Retryable() bool {
	return e.Kind == KindThrottled || e.Kind == KindUpstream || e.Kind == KindTransport
}

// Client talks to one http adapter.
type Client struct {
	ID      string
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient builds a client with the redirect policy this protocol requires.
func NewClient(id, baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		ID: id, BaseURL: strings.TrimRight(baseURL, "/"), Token: token,
		HTTP: &http.Client{
			Timeout: timeout,
			// **Redirects are refused, not followed.** This is the dynamic half
			// of the SSRF story and the static base_url check cannot cover it:
			// an operator-approved public adapter can answer 302 and point at
			// 169.254.169.254, Go follows up to ten hops by default, and the
			// response body travels days[].text → the model → the user's chat.
			// One config line and one redirect is a complete read-and-exfiltrate
			// path.
			//
			// Refusing costs nothing legitimate: /v0/weather is a fixed path
			// this protocol defines, so a redirect from it means the base_url is
			// wrong — which is what the error says.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Do performs one protocol call: POST body to path, decode into out.
//
// Retries live in the caller, not here, so that a deadline is spent once by
// somebody who knows the budget. docs/specs/transport.md tells adapters not to
// retry for the same reason — the layer that knows how long it has is the only
// one that can decide.
func (c *Client) Do(ctx context.Context, path string, body, out any) error {
	u, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return c.fail(KindRequest, 0, "bad path "+path, err)
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return c.fail(KindRequest, 0, "encode request", err)
		}
		rdr = bytes.NewReader(b)
	}
	method := http.MethodPost
	if body == nil {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return c.fail(KindRequest, 0, "build request", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if id, ok := ctx.Value(requestIDKey).(string); ok && id != "" {
		req.Header.Set("X-Daycore-Request-Id", id)
	}
	// Tell the adapter how long it has, so it can decline to start work it
	// cannot finish. Advisory — we enforce with the context regardless.
	if dl, ok := ctx.Deadline(); ok {
		if ms := time.Until(dl).Milliseconds(); ms > 0 {
			req.Header.Set("X-Daycore-Deadline-Ms", strconv.FormatInt(ms, 10))
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return c.fail(KindTransport, 0, "call "+u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return c.fail(KindProtocol, resp.StatusCode,
			"adapter redirected; base_url points at the wrong place — redirects are not followed", nil)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Bounded read: an adapter answering an error with a gigabyte of HTML
		// should not cost this process the memory to hold it.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		// The URL goes in the DETAIL, always — including on a status-coded
		// failure, where the first version carried only the adapter's own words.
		// "location is required" with no address in it is not something anybody
		// can act on when three sources are configured. Detail() is log-only, so
		// this is the one place a URL is safe to keep.
		return c.fail(kindForStatus(resp.StatusCode), resp.StatusCode,
			u+": "+strings.TrimSpace(string(msg)), nil)
	}
	if out == nil {
		return nil
	}
	// Also bounded: the protocol's payloads are small, and a body limit is the
	// only defence against an adapter that streams forever.
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := dec.Decode(out); err != nil {
		return c.fail(KindProtocol, resp.StatusCode, "decode response", err)
	}
	return nil
}

// maxResponseBytes bounds one adapter reply. Generous for a forecast or five
// search hits, small enough that a hostile adapter cannot exhaust memory.
const maxResponseBytes = 2 << 20

func kindForStatus(code int) ErrKind {
	switch {
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return KindAuth
	case code == http.StatusNotFound:
		return KindNotFound
	case code == http.StatusTooManyRequests:
		return KindThrottled
	case code >= 500:
		return KindUpstream
	case code >= 400:
		return KindRequest
	}
	return KindProtocol
}

func (c *Client) fail(kind ErrKind, status int, what string, cause error) error {
	detail := what
	if cause != nil {
		detail += ": " + cause.Error()
	}
	return &Error{Kind: kind, Source: c.ID, Status: status, detail: detail}
}

type ctxKey int

const requestIDKey ctxKey = iota

// WithRequestID puts the incoming request id on the context so adapter calls
// carry it. Cross-process debugging has nothing else to correlate on.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// AsError extracts an *Error, for callers deciding what to log.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
