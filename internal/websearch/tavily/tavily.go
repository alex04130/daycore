// Package tavily is the Tavily web search engine, registered as "tavily".
//
// A key is required; websearch.buildEngine refuses the source when the variable
// is empty rather than constructing something that fails every call. That is a
// behaviour change from the code this came out of, which fell through to
// DuckDuckGo whenever Tavily was unset OR failed — so a deployment with an
// expired key silently searched somewhere else and nothing said so.
package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"daycore/internal/websearch"
)

const defaultURL = "https://api.tavily.com/search"

func init() {
	websearch.Register("tavily", func(c websearch.Config) websearch.Engine {
		url := c.BaseURL
		if url == "" {
			url = defaultURL
		}
		return &engine{key: c.APIKey, url: url, http: c.HTTP}
	})
}

type engine struct {
	key  string
	url  string
	http *http.Client
}

func (c *engine) Name() string { return "tavily" }

type tavilyReq struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results"`
	SearchDepth string `json:"search_depth"`
}

func (c *engine) Search(ctx context.Context, query string, maxResults int) ([]websearch.Result, error) {
	body, err := json.Marshal(tavilyReq{APIKey: c.key, Query: query, MaxResults: maxResults, SearchDepth: "basic"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
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
	out := make([]websearch.Result, 0, len(tr.Results))
	for _, r := range tr.Results {
		if len(out) == maxResults {
			break
		}
		out = append(out, websearch.Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}
