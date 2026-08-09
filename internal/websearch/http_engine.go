package websearch

import (
	"context"

	"daycore/internal/adapters"
)

// httpEngine reaches an external search adapter over the protocol in
// docs/specs/provider-protocol.md.
//
// Small on purpose, the same as the weather side: the claim the protocol makes
// is that the layers above cannot tell an adapter from a compiled-in engine, so
// everything that is not "translate a result list" belongs in
// internal/adapters, where four capabilities share it.
type httpEngine struct {
	client *adapters.Client
	id     string
}

func (e *httpEngine) Name() string { return e.id }

type searchReq struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
	Locale string `json:"locale"`
}

type searchResp struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	} `json:"results"`
}

func (e *httpEngine) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	var r searchResp
	if err := e.client.Do(ctx, "/v0/search", searchReq{Query: query, Limit: maxResults}, &r); err != nil {
		return nil, err
	}
	// An empty result list is NOT an error here, unlike an empty forecast.
	//
	// The difference is what the caller can do with it: "no results for this
	// query" is a true and useful answer that the model can act on, whereas an
	// empty forecast renders as an ordinary sentence with the weather missing.
	// Treating a legitimate no-match as a source failure would mark a working
	// search engine unhealthy for asking about something obscure.
	out := make([]Result, 0, len(r.Results))
	for _, hit := range r.Results {
		if len(out) == maxResults {
			break
		}
		out = append(out, Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Snippet})
	}
	return out, nil
}
