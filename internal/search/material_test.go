package search

import (
	"context"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// fakeStore stands in for a store WITHOUT a native full-text index, forcing
// the substring fallback — the recall floor every backend must keep even
// when the index is missing, the term is too short, or the engine cannot
// tokenize the script. Embedding the domain.Store interface gives the
// assertion "does this implement MaterialFTS" a false answer without
// spelling out 179 methods.
type fakeStore struct{ domain.Store }

type fakeMaterials struct{ domain.MaterialRepository }

func (f fakeMaterials) List(_ interface{}, sid, category, query string, limit, offset int) ([]domain.Material, error) {
	if sid != "s1" {
		return nil, domain.ErrNotFound
	}
	all := []domain.Material{
		{ID: "m1", Title: "离散数学笔记", Summary: "", Body: "第三章是考试重点，包含图论与集合论。"},
		{ID: "m2", Title: "跑步计划", Summary: "每周三次晨跑", Body: ""},
		{ID: "m3", Title: "", Summary: "", Body: "没有任何可摘要的内容"},
	}
	out := make([]domain.Material, 0, len(all))
	for _, m := range all {
		if query != "" && !strings.Contains(strings.ToLower(m.Title+m.Summary+m.Body), strings.ToLower(query)) {
			continue
		}
		out = append(out, m)
	}
	// apply offset/limit like the real repositories
	if offset > len(out) {
		return []domain.Material{}, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (f fakeStore) Materials() domain.MaterialRepository { return fakeMaterials{} }

func TestFallbackSearchAndSnippet(t *testing.T) {
	s := NewMaterialSearcher(fakeStore{})
	ctx := context.Background()

	res, err := s.Search(ctx, "s1", domain.SearchQuery{Term: "图论", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].ID != "m1" {
		t.Fatalf("fallback search must find the substring match, got %+v", res)
	}
	if res[0].Score != 1 {
		t.Errorf("fallback results all score 1 (the recall floor), got %v", res[0].Score)
	}
	// m1 has no summary, so the snippet is the body cut around the term.
	if !strings.Contains(res[0].Snippet, "图论") {
		t.Errorf("snippet must be cut around the term, got %q", res[0].Snippet)
	}
	if strings.Contains(res[0].Snippet, "每周三次") {
		t.Errorf("snippet must come from the matching material, got %q", res[0].Snippet)
	}
}

func TestBuildSnippetMatrix(t *testing.T) {
	long := strings.Repeat("很长的正文", 30)
	cases := []struct{ name, summary, body, term, wantContains string }{
		{"summary wins", "摘要内容", "正文内容", "正文", "摘要内容"},
		{"term found mid-body", "", "前缀前缀前缀图论后缀后缀", "图论", "图论"},
		{"term at start clamps", "", "图论在开头出现", "图论", "图论"},
		{"term absent takes head", "", "没有这个关键词的正文", "不存在", "没有这个关键词"},
		{"empty everything", "", "", "", ""},
		{"truncates at 160 runes", "", long, "", "很长的正文"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSnippet(tc.summary, tc.body, tc.term)
			if !strings.Contains(got, tc.wantContains) {
				t.Errorf("buildSnippet(%q,%q,%q) = %q, want it to contain %q",
					tc.summary, tc.body, tc.term, got, tc.wantContains)
			}
			if len([]rune(got)) > 161 {
				t.Errorf("snippet longer than 160 runes: %d", len([]rune(got)))
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 3, "abc"},
		{"abcdef", 3, "abc…"},
		{"中文测试", 2, "中文…"},
		{"", 0, ""},
	}
	for _, tc := range cases {
		if got := truncateRunes(tc.in, tc.n); got != tc.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestIndexDeindexAreNoOps(t *testing.T) {
	s := NewMaterialSearcher(fakeStore{})
	if err := s.Index(context.Background(), "s", "id", "t", "b"); err != nil {
		t.Errorf("Index must be a no-op, got %v", err)
	}
	if err := s.Deindex(context.Background(), "id"); err != nil {
		t.Errorf("Deindex must be a no-op, got %v", err)
	}
}
