package schedule

import (
	"testing"
	"time"

	"daycore/internal/domain"
)

func blk(id, t string, dur int) domain.TimeBlock {
	return domain.TimeBlock{ID: id, Title: id, Time: hm(t), DurationMin: mn(dur)}
}

func TestOverlaps(t *testing.T) {
	loc := time.UTC
	const date = "2026-03-10"

	cases := []struct {
		name   string
		blocks []domain.TimeBlock
		want   [][2]string
	}{
		{
			name:   "背靠背不算冲突 —— 一天本来就该长这样",
			blocks: []domain.TimeBlock{blk("a", "09:00", 60), blk("b", "10:00", 60)},
		},
		{
			name:   "真重叠",
			blocks: []domain.TimeBlock{blk("a", "09:00", 90), blk("b", "10:00", 60)},
			want:   [][2]string{{"a", "b"}},
		},
		{
			name:   "一个套着另一个",
			blocks: []domain.TimeBlock{blk("outer", "09:00", 180), blk("inner", "10:00", 30)},
			want:   [][2]string{{"outer", "inner"}},
		},
		{
			name:   "三个互相压 —— 三对，不是一对",
			blocks: []domain.TimeBlock{blk("a", "09:00", 120), blk("b", "09:30", 120), blk("c", "10:00", 120)},
			want:   [][2]string{{"a", "b"}, {"a", "c"}, {"b", "c"}},
		},
		{
			name: "墓碑不与任何东西冲突 —— 用户已经说了这次不去",
			blocks: []domain.TimeBlock{
				blk("a", "09:00", 90),
				func() domain.TimeBlock { b := blk("gone", "09:30", 60); b.Hidden = true; return b }(),
			},
		},
		{
			name:   "没有时段的事不占分钟",
			blocks: []domain.TimeBlock{blk("a", "09:00", 90), {ID: "loose", Title: "想想论文"}},
		},
		{
			name:   "零时长不占分钟",
			blocks: []domain.TimeBlock{blk("a", "09:00", 90), blk("z", "09:30", 0)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Overlaps(c.blocks, date, loc)
			if len(got) != len(c.want) {
				t.Fatalf("found %d overlaps, want %d: %+v", len(got), len(c.want), got)
			}
			for i, w := range c.want {
				if got[i].A.ID != w[0] || got[i].B.ID != w[1] {
					t.Errorf("pair %d = (%s,%s), want (%s,%s)", i, got[i].A.ID, got[i].B.ID, w[0], w[1])
				}
			}
		})
	}
}

// The contested span is what a decision card would have to show — "these two
// both want 10:00–10:30" is actionable, "these two clash" is not.
func TestOverlapReportsTheContestedSpan(t *testing.T) {
	loc := time.UTC
	got := Overlaps([]domain.TimeBlock{blk("a", "09:00", 90), blk("b", "10:00", 60)}, "2026-03-10", loc)
	if len(got) != 1 {
		t.Fatalf("want one overlap, got %d", len(got))
	}
	if h, m := got[0].Start.Hour(), got[0].Start.Minute(); h != 10 || m != 0 {
		t.Errorf("contested start = %02d:%02d, want 10:00", h, m)
	}
	if h, m := got[0].End.Hour(), got[0].End.Minute(); h != 10 || m != 30 {
		t.Errorf("contested end = %02d:%02d, want 10:30", h, m)
	}
}
