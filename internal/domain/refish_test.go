package domain

import "testing"

func TestRefishable(t *testing.T) {
	dur := 30
	base := TimeBlock{ID: "b", Title: "跑步", Type: BlockTask, DurationMin: &dur}

	cases := []struct {
		name string
		mut  func(*TimeBlock)
		want bool
	}{
		{"没做完的任务可以补钓", func(*TimeBlock) {}, true},
		{"做完了就不必再提", func(b *TimeBlock) { b.Completed = true }, false},
		// 预约的时间本身就是重点 —— 错过的课就是错过了，再排一次是在骗人。
		{"预约不补钓", func(b *TimeBlock) { b.Type = BlockAppointment }, false},
		{"成就是记录不是计划", func(b *TimeBlock) { b.IsAchievement = true }, false},
		{"到达上限就不再问", func(b *TimeBlock) { b.RescheduleCount = RescheduleCap }, false},
		{"上限之内还可以", func(b *TimeBlock) { b.RescheduleCount = RescheduleCap - 1 }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := base
			c.mut(&b)
			if got := b.Refishable(); got != c.want {
				t.Errorf("Refishable() = %v, want %v", got, c.want)
			}
		})
	}
}

// The chain points at what the user wanted, not at the previous attempt —
// otherwise answering "what happened to that" means walking a linked list.
func TestRescheduleChainKeepsTheRoot(t *testing.T) {
	first := TimeBlock{ID: "orig"}
	root, n := RescheduleChain(first)
	if root != "orig" || n != 1 {
		t.Fatalf("first retry: root=%q n=%d, want orig/1", root, n)
	}

	second := TimeBlock{ID: "try1", RescheduledFrom: root, RescheduleCount: n}
	root, n = RescheduleChain(second)
	if root != "orig" || n != 2 {
		t.Fatalf("second retry: root=%q n=%d, want orig/2", root, n)
	}

	third := TimeBlock{ID: "try2", RescheduledFrom: root, RescheduleCount: n}
	root, n = RescheduleChain(third)
	if root != "orig" || n != 3 {
		t.Errorf("third retry: root=%q n=%d, want orig/3", root, n)
	}
	// And that one is the last: the block it produces is at the cap.
	capped := TimeBlock{ID: "try3", Type: BlockTask, RescheduledFrom: root, RescheduleCount: n}
	if capped.Refishable() {
		t.Error("a block at the cap should not be offered again")
	}
}
