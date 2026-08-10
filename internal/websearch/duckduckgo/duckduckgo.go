// Package duckduckgo scrapes DuckDuckGo's plain-HTML endpoint, registered as
// "duckduckgo".
//
// No key, which is what makes it the source a deployment always has. It is a
// scraper, so the parser is deliberately lenient: a layout change yields an
// empty result slice rather than an error, because "I found nothing" is a
// truthful answer to give a model and a parse error is not.
package duckduckgo

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"daycore/internal/websearch"
)

const (
	defaultURL = "https://html.duckduckgo.com/html/"
	// DDG serves the plain-HTML page only to browser-looking clients.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

func init() {
	websearch.Register("duckduckgo", func(c websearch.Config) websearch.Engine {
		url := c.BaseURL
		if url == "" {
			url = defaultURL
		}
		return &engine{url: url, http: c.HTTP}
	})
}

type engine struct {
	url  string
	http *http.Client
}

func (c *engine) Name() string { return "duckduckgo" }

func (c *engine) Search(ctx context.Context, query string, maxResults int) ([]websearch.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"?q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
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
func parseDDG(page string, maxResults int) []websearch.Result {
	anchors := ddgAnchorRe.FindAllStringSubmatchIndex(page, -1)
	out := []websearch.Result{}
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
		out = append(out, websearch.Result{Title: title, URL: href, Snippet: snippet})
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
