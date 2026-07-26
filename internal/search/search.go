// Package search provides web search for the companion agent loop: Tavily
// when an API key is configured, degrading to scraping DuckDuckGo's plain-HTML
// endpoint (no key needed) when it isn't or when the Tavily request fails.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultTavilyURL = "https://api.tavily.com/search"
	defaultDDGURL    = "https://html.duckduckgo.com/html/"
	// DDG serves the plain-HTML page only to browser-looking clients.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Result is one search hit.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Client performs web searches. Fields are only read, never mutated, by
// Search — override them before first use (tests inject httptest URLs).
type Client struct {
	HTTP      *http.Client
	TavilyKey string
	TavilyURL string
	DDGURL    string
}

// New returns a Client with the public endpoints, an 8s timeout, and the
// Tavily key from TAVILY_API_KEY (empty → DuckDuckGo only).
func New() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 8 * time.Second},
		TavilyKey: os.Getenv("TAVILY_API_KEY"),
		TavilyURL: defaultTavilyURL,
		DDGURL:    defaultDDGURL,
	}
}

// Search returns up to maxResults hits (clamped to 1..5) for query, using
// Tavily when a key is set and falling back to DuckDuckGo on any Tavily error.
func (c *Client) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search: empty query")
	}
	switch {
	case maxResults < 1:
		maxResults = 1
	case maxResults > 5:
		maxResults = 5
	}
	if c.TavilyKey != "" {
		if rs, err := c.tavily(ctx, query, maxResults); err == nil {
			return rs, nil
		}
	}
	return c.duckduckgo(ctx, query, maxResults)
}

// ─── Tavily ──────────────────────────────────────────────────────────────────

type tavilyReq struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results"`
	SearchDepth string `json:"search_depth"`
}

func (c *Client) tavily(ctx context.Context, query string, maxResults int) ([]Result, error) {
	body, err := json.Marshal(tavilyReq{APIKey: c.TavilyKey, Query: query, MaxResults: maxResults, SearchDepth: "basic"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TavilyURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search: tavily http %d", resp.StatusCode)
	}
	var tr struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("search: tavily decode: %w", err)
	}
	out := make([]Result, 0, len(tr.Results))
	for _, r := range tr.Results {
		if len(out) == maxResults {
			break
		}
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}

// ─── DuckDuckGo ──────────────────────────────────────────────────────────────

func (c *Client) duckduckgo(ctx context.Context, query string, maxResults int) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.DDGURL+"?q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search: duckduckgo http %d", resp.StatusCode)
	}
	return parseDDG(string(data), maxResults), nil
}

var (
	ddgAnchorRe  = regexp.MustCompile(`(?s)<a\s[^>]*class="[^"]*\bresult__a\b[^"]*"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?s)class="[^"]*\bresult__snippet\b[^"]*"[^>]*>(.*?)</(?:a|div)>`)
	hrefRe       = regexp.MustCompile(`href="([^"]*)"`)
	tagRe        = regexp.MustCompile(`<[^>]*>`)
)

// parseDDG is deliberately lenient: layout drift yields an empty slice, never
// an error. Each anchor's snippet is looked up between it and the next anchor.
func parseDDG(page string, maxResults int) []Result {
	anchors := ddgAnchorRe.FindAllStringSubmatchIndex(page, -1)
	out := []Result{}
	for i, m := range anchors {
		if len(out) == maxResults {
			break
		}
		href := ""
		if hm := hrefRe.FindStringSubmatch(page[m[0]:m[2]]); hm != nil {
			href = decodeHref(hm[1])
		}
		if strings.Contains(href, "duckduckgo.com/y.js") { // ad slot
			continue
		}
		title := stripTags(page[m[2]:m[3]])
		if title == "" && href == "" {
			continue
		}
		end := len(page)
		if i+1 < len(anchors) {
			end = anchors[i+1][0]
		}
		snippet := ""
		if sm := ddgSnippetRe.FindStringSubmatch(page[m[1]:end]); sm != nil {
			snippet = stripTags(sm[1])
		}
		out = append(out, Result{Title: title, URL: href, Snippet: snippet})
	}
	return out
}

// decodeHref resolves DDG's /l/?uddg=<escaped real url>&rut=… redirect links.
func decodeHref(href string) string {
	href = html.UnescapeString(href)
	if i := strings.Index(href, "uddg="); i >= 0 {
		v := href[i+len("uddg="):]
		if j := strings.IndexByte(v, '&'); j >= 0 {
			v = v[:j]
		}
		if u, err := url.QueryUnescape(v); err == nil && u != "" {
			return u
		}
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	return href
}

func stripTags(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}
