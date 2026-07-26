package server

import "testing"

func TestApplyPlanAction(t *testing.T) {
	blocks := []map[string]any{
		{"id": "1", "time": "09:00", "title": "A"},
	}

	// add — new block, sorted by time (08:00 before 09:00)
	blocks = applyPlanAction(blocks, planAction{
		Action: "add",
		Block:  map[string]any{"time": "08:00", "title": "B"},
	})
	if len(blocks) != 2 {
		t.Fatalf("after add: len=%d", len(blocks))
	}
	if blocks[0]["title"] != "B" {
		t.Fatalf("add should sort by time; first=%v", blocks[0]["title"])
	}
	if _, ok := blocks[0]["id"]; !ok {
		t.Fatal("added block should get an id")
	}

	// update — change A's time by matching its title
	blocks = applyPlanAction(blocks, planAction{
		Action:  "update",
		Match:   map[string]any{"title": "A"},
		Changes: map[string]any{"time": "10:00"},
	})
	var aTime any
	for _, b := range blocks {
		if b["title"] == "A" {
			aTime = b["time"]
		}
	}
	if aTime != "10:00" {
		t.Fatalf("update failed; A.time=%v", aTime)
	}

	// remove — drop B
	blocks = applyPlanAction(blocks, planAction{
		Action: "remove",
		Match:  map[string]any{"title": "B"},
	})
	if len(blocks) != 1 || blocks[0]["title"] != "A" {
		t.Fatalf("remove failed; blocks=%v", blocks)
	}
}
