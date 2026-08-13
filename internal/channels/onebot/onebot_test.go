package onebot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"daycore/internal/channels"
)

// Stop used to be `close(a.stop)` — a double stop (a shutdown path running
// twice) or a stop before Start closed a nil or already-closed channel and
// panicked. Both are ordinary shutdown shapes, not operator errors.
func TestStopTwiceDoesNotPanic(t *testing.T) {
	a := New(Config{WSURL: "ws://127.0.0.1:1"})
	if err := a.Stop(); err != nil { // before Start: stop is nil
		t.Fatalf("stop before start must be a no-op, got %v", err)
	}
	if err := a.Start(context.Background(), make(chan channels.InboundMsg, 1)); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("second stop must be a no-op, got %v", err)
	}
}

func TestRateLimitWindow(t *testing.T) {
	a := New(Config{})
	user := "1001"
	for i := 0; i < 10; i++ {
		if a.rateLimit(user) {
			t.Fatalf("message %d within the window was blocked", i+1)
		}
	}
	if !a.rateLimit(user) {
		t.Fatal("the 11th message in one window must be blocked")
	}
	// A different user has their own window.
	if a.rateLimit("1002") {
		t.Fatal("a different user must not inherit the rate limit")
	}
	// The window opens again once it is older than 60s.
	a.mu.Lock()
	a.lastMsg[user] = time.Now().Add(-61 * time.Second)
	a.mu.Unlock()
	if a.rateLimit(user) {
		t.Fatal("a stale window must open a fresh one, not block")
	}
}

func TestMentionsBot(t *testing.T) {
	mk := func(segType, qq string, self int64, raw string) onebotEvent {
		ev := onebotEvent{SelfID: self, RawMessage: raw}
		ev.Message = append(ev.Message, struct {
			Type string `json:"type"`
			Data struct {
				Text string `json:"text"`
				URL  string `json:"url"`
				File string `json:"file"`
				QQ   string `json:"qq"`
			} `json:"data"`
		}{Type: segType, Data: struct {
			Text string `json:"text"`
			URL  string `json:"url"`
			File string `json:"file"`
			QQ   string `json:"qq"`
		}{QQ: qq}})
		return ev
	}
	if !mentionsBot(mk("at", "42", 42, "")) {
		t.Error("an at segment naming the bot must count")
	}
	if !mentionsBot(mk("at", "all", 42, "")) {
		t.Error("@all must count")
	}
	if mentionsBot(mk("at", "7", 42, "")) {
		t.Error("an at segment naming somebody else must not count")
	}
	// Fallback on the CQ-code raw string, for adapters that only send that.
	if !mentionsBot(mk("text", "", 42, "hi [CQ:at,qq=42]")) {
		t.Error("the raw CQ-code fallback must count")
	}
	// SelfID 0 only disables the RAW fallback: the segment loop compares
	// against the string "0" and a qq=0 at segment still matches. Locked as
	// the current behaviour — in practice OneBot always sends self_id > 0, and
	// qq=0 segments are malformed clients; changing either half here would
	// need the other reviewed together.
	if !mentionsBot(mk("at", "0", 0, "[CQ:at,qq=0]")) {
		t.Error("segment match against qq=0 currently counts (locked behaviour)")
	}
	if mentionsBot(mk("text", "", 0, "[CQ:at,qq=0]")) {
		t.Error("SelfID 0 must disable the raw fallback")
	}
}

func TestHandleFrameFiltering(t *testing.T) {
	inbound := make(chan channels.InboundMsg, 4)
	a := New(Config{WSURL: "ws://127.0.0.1:1"})
	a.inbound = inbound
	ctx := context.Background()

	// Malformed JSON is dropped, never a panic.
	a.handleFrame(ctx, []byte("{not json"))
	// Non-message post_types are ignored.
	a.handleFrame(ctx, []byte(`{"post_type":"notice"}`))
	// A group message that neither mentions the bot nor starts with / is
	// ignored — the classic "wake word" filter.
	a.handleFrame(ctx, []byte(`{"post_type":"message","message_type":"group","self_id":42,"user_id":7,"message":[{"type":"text","data":{"text":"hello"}}],"raw_message":"hello","sender":{"user_id":7,"nickname":"N"}}`))
	// A group command passes.
	a.handleFrame(ctx, []byte(`{"post_type":"message","message_type":"group","self_id":42,"user_id":7,"raw_message":"/plan","sender":{"user_id":7,"nickname":"N"}}`))
	// A private message passes and carries its text and image.
	a.handleFrame(ctx, []byte(`{"post_type":"message","message_type":"private","self_id":42,"user_id":7,"raw_message":"raw","sender":{"user_id":7,"nickname":"N"},"message":[{"type":"text","data":{"text":"hi"}},{"type":"image","data":{"url":"http://x/1.png","file":"1.png"}}]}`))
	if len(inbound) != 2 {
		t.Fatalf("expected 2 inbound messages, got %d", len(inbound))
	}
	m1, m2 := <-inbound, <-inbound
	if m1.Text != "/plan" && m2.Text != "/plan" {
		t.Fatal("the group command must pass through with its command text")
	}
	var withImg channels.InboundMsg
	if m1.Text == "hi" {
		withImg = m1
	} else {
		withImg = m2
	}
	if len(withImg.Attachments) != 1 || withImg.Attachments[0].URL != "http://x/1.png" {
		t.Errorf("the image attachment must be carried, got %+v", withImg.Attachments)
	}
	// An unbound user is filtered out by the binding check.
	a.cfg.ValidateBinding = func(context.Context, string) bool { return false }
	a.handleFrame(ctx, []byte(`{"post_type":"message","message_type":"private","self_id":42,"user_id":7,"raw_message":"x","sender":{"user_id":7,"nickname":"N"}}`))
	if len(inbound) != 0 {
		t.Fatal("an unbound user must not reach the inbound queue")
	}
}

func TestSendAndSegments(t *testing.T) {
	a := New(Config{})
	if err := a.Send(context.Background(), "u", channels.Text("x")); err == nil {
		t.Fatal("Send without a connection must fail")
	}
	// segments is the pure half: text, base64 fallback, record for audio,
	// and unknown kinds produce nothing rather than a bogus segment.
	segs := a.segments(channels.Outbound{Text: "hi", Attachments: []channels.OutAttachment{
		{Kind: "image", Data: []byte{1, 2, 3}},
		{Kind: "audio", URL: "http://x/a.mp3", MIME: "audio/mpeg", Name: "a.mp3"},
		{Kind: "file", URL: "http://x/f.pdf"},
	}})
	if len(segs) != 3 {
		t.Fatalf("want text + image + audio segments, got %d: %+v", len(segs), segs)
	}
	if segs[0]["type"] != "text" {
		t.Errorf("segment 0 must be text, got %v", segs[0]["type"])
	}
	img := segs[1]["data"].(map[string]any)["file"].(string)
	if img != "base64://"+base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Errorf("image bytes must be base64-prefixed, got %q", img)
	}
	if segs[2]["type"] != "record" {
		t.Errorf("audio must map to the OneBot record segment, got %v", segs[2]["type"])
	}
	// An empty outbound yields no segments — nothing to say and nothing to show.
	if segs := a.segments(channels.Outbound{}); len(segs) != 0 {
		t.Errorf("empty outbound must produce no segments, got %d", len(segs))
	}
	// An attachment with neither URL nor data is nil (and logged), not a
	// broken segment.
	if seg := a.segment(channels.OutAttachment{Kind: "image"}); seg != nil {
		t.Errorf("an empty attachment must produce nil, got %+v", seg)
	}
}

func TestFeatures(t *testing.T) {
	f := New(Config{}).Features()
	if !f.Images || !f.Audio || f.Files || f.Markdown {
		t.Errorf("unexpected feature set: %+v", f)
	}
}

// The whole frame path round-trips through JSON, not just the struct — a
// field rename in the struct would otherwise silently break parsing.
func TestHandleFrameDecodesRealWireShape(t *testing.T) {
	inbound := make(chan channels.InboundMsg, 1)
	a := New(Config{})
	a.inbound = inbound
	raw, err := json.Marshal(map[string]any{
		"post_type":    "message",
		"message_type": "private",
		"self_id":      42,
		"user_id":      7,
		"raw_message":  "hello",
		"sender":       map[string]any{"user_id": 7, "nickname": "N"},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.handleFrame(context.Background(), raw)
	select {
	case m := <-inbound:
		if m.Channel != "onebot" || m.ExternalID != "7" || m.DisplayName != "N" || m.Text != "hello" {
			t.Errorf("decoded %+v", m)
		}
	default:
		t.Fatal("the frame was not delivered")
	}
}
