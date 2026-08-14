package onebot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"daycore/internal/channels"

	"github.com/gorilla/websocket"
)

type Config struct {
	WSURL           string
	Token           string
	Log             *slog.Logger
	ValidateBinding func(ctx context.Context, externalID string) bool
}

type Adapter struct {
	cfg       Config
	conn      *websocket.Conn
	mu        sync.Mutex
	inbound   chan<- channels.InboundMsg
	stop      chan struct{}
	reconnect time.Duration
	lastMsg   map[string]time.Time
	msgCount  map[string]int
}

func New(cfg Config) *Adapter {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Adapter{
		cfg:      cfg,
		lastMsg:  map[string]time.Time{},
		msgCount: map[string]int{},
	}
}

func (a *Adapter) Name() string  { return "onebot" }
func (a *Adapter) Label() string { return "QQ (OneBot/NapCat)" }

func (a *Adapter) Start(ctx context.Context, inbound chan<- channels.InboundMsg) error {
	a.inbound = inbound
	a.stop = make(chan struct{})
	a.reconnect = 1 * time.Second
	go a.loop(ctx)
	return nil
}

// Stop is idempotent and safe to call before Start: a shutdown path that
// stops a registry twice — or one that stops a registry whose adapter never
// got started — must not panic on a double close of a nil or closed channel.
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stop == nil {
		return nil // never started; nothing to stop
	}
	select {
	case <-a.stop:
		return nil // already stopped
	default:
	}
	close(a.stop)
	return nil
}

func (a *Adapter) loop(ctx context.Context) {
	for {
		select {
		case <-a.stop:
			return
		default:
		}
		if err := a.connectAndRead(ctx); err != nil {
			a.cfg.Log.Error("onebot disconnected", "err", err)
		}
		select {
		case <-a.stop:
			return
		case <-time.After(a.reconnect):
		}
		a.reconnect = a.reconnect * 2
		if a.reconnect > 60*time.Second {
			a.reconnect = 60 * time.Second
		}
	}
}

func (a *Adapter) connectAndRead(ctx context.Context) error {
	header := http.Header{}
	if a.cfg.Token != "" {
		header.Set("Authorization", "Bearer "+a.cfg.Token)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, a.cfg.WSURL, header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()
	a.reconnect = 1 * time.Second

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		a.handleFrame(ctx, msg)
	}
}

func (a *Adapter) rateLimit(externalID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	start, ok := a.lastMsg[externalID]
	if !ok || now.Sub(start) > 60*time.Second {
		// Opening a new window: evict stale entries so the maps only ever hold
		// senders seen in the last minute (bounded growth).
		for id, t := range a.lastMsg {
			if now.Sub(t) > 60*time.Second {
				delete(a.lastMsg, id)
				delete(a.msgCount, id)
			}
		}
		a.msgCount[externalID] = 1
		a.lastMsg[externalID] = now
		return false
	}
	a.msgCount[externalID]++
	if a.msgCount[externalID] > 10 {
		return true
	}
	return false
}

type onebotEvent struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	SubType     string `json:"sub_type"`
	SelfID      int64  `json:"self_id"`
	UserID      int64  `json:"user_id"`
	GroupID     int64  `json:"group_id"`
	RawMessage  string `json:"raw_message"`
	Sender      struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
	} `json:"sender"`
	Message []struct {
		Type string `json:"type"`
		Data struct {
			Text string `json:"text"`
			URL  string `json:"url"`
			File string `json:"file"`
			QQ   string `json:"qq"` // target of an "at" segment
		} `json:"data"`
	} `json:"message"`
}

// mentionsBot reports whether a group message actually @-mentions the bot (or
// @all), rather than merely containing an "@" character anywhere.
func mentionsBot(ev onebotEvent) bool {
	self := fmt.Sprintf("%d", ev.SelfID)
	for _, seg := range ev.Message {
		if seg.Type == "at" && (seg.Data.QQ == self || seg.Data.QQ == "all") {
			return true
		}
	}
	// Fallback for adapters that only send the CQ-code raw string.
	return ev.SelfID != 0 && strings.Contains(ev.RawMessage, fmt.Sprintf("[CQ:at,qq=%d]", ev.SelfID))
}

func (a *Adapter) handleFrame(ctx context.Context, raw []byte) {
	var ev onebotEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return
	}
	if ev.PostType != "message" {
		return
	}

	externalID := fmt.Sprintf("%d", ev.UserID)

	// Group chat filtering: only process real @-mentions of the bot or /commands.
	if ev.MessageType == "group" {
		hasCmd := strings.HasPrefix(strings.TrimSpace(ev.RawMessage), "/")
		if !mentionsBot(ev) && !hasCmd {
			return
		}
	}

	// Binding check: only process messages from bound users.
	if a.cfg.ValidateBinding != nil && !a.cfg.ValidateBinding(ctx, externalID) {
		return
	}

	// Rate limit: max 10 messages per 60 seconds per user.
	if a.rateLimit(externalID) {
		_ = a.Send(ctx, externalID, channels.Text("消息太频繁，请稍后再试"))
		return
	}

	text := ev.RawMessage
	var atts []channels.Attachment
	for _, seg := range ev.Message {
		if seg.Type == "text" {
			text = seg.Data.Text
		} else if seg.Type == "image" && seg.Data.URL != "" {
			atts = append(atts, channels.Attachment{URL: seg.Data.URL, MimeType: "image/unknown", Name: seg.Data.File})
		}
	}
	select {
	case a.inbound <- channels.InboundMsg{
		Channel:     "onebot",
		ExternalID:  externalID,
		DisplayName: ev.Sender.Nickname,
		Text:        text,
		Attachments: atts,
	}:
	default:
	}
}

// Features reports what OneBot can carry.
//
// Images and audio are OneBot 11 message segments; files are not (the standard
// has no private-message file segment, implementations that offer one do it
// through their own extension API). Markdown is plain text on QQ — sending it
// shows the asterisks.
func (a *Adapter) Features() channels.Features {
	return channels.Features{
		Images: true,
		Audio:  true,
		Files:  false,
		// QQ renders markdown literally, so a reply with ** in it reads worse
		// than the same reply without.
		Markdown: false,
	}
}

func (a *Adapter) Send(ctx context.Context, externalID string, msg channels.Outbound) error {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	segments := a.segments(msg)
	if len(segments) == 0 {
		return nil // nothing to say and nothing to show
	}
	payload := map[string]any{
		"action": "send_private_msg",
		"params": map[string]any{
			"user_id": externalID,
			"message": segments,
		},
	}
	return conn.WriteJSON(payload)
}

// segments builds the OneBot 11 message array.
//
// The array form rather than the plain string: a string can only be text, and
// mixing text with an image is the normal case (here is your day / here is the
// picture of it). OneBot accepts either, and the array is a superset.
func (a *Adapter) segments(msg channels.Outbound) []map[string]any {
	var out []map[string]any
	if msg.Text != "" {
		out = append(out, map[string]any{
			"type": "text",
			"data": map[string]any{"text": msg.Text},
		})
	}
	for _, at := range msg.Attachments {
		seg := a.segment(at)
		if seg == nil {
			// Reporting rather than dropping: a caller that sent an attachment
			// believed the user would see it, and a reply referring to a picture
			// nobody received is worse than one that never mentioned it.
			a.cfg.Log.Warn("onebot: cannot carry attachment", "kind", at.Kind, "mime", at.MIME)
			continue
		}
		out = append(out, seg)
	}
	return out
}

func (a *Adapter) segment(at channels.OutAttachment) map[string]any {
	// OneBot takes a `file` field that may be a URL, a path, or base64 with this
	// exact prefix. Base64 is what a local file bus can offer without exposing a
	// public URL, so it is the fallback rather than the exception.
	file := at.URL
	if file == "" && len(at.Data) > 0 {
		file = "base64://" + base64.StdEncoding.EncodeToString(at.Data)
	}
	if file == "" {
		return nil
	}
	switch at.Kind {
	case "image":
		return map[string]any{"type": "image", "data": map[string]any{"file": file}}
	case "audio":
		// OneBot calls it "record". The name is a historical accident of the
		// protocol, not a different thing.
		return map[string]any{"type": "record", "data": map[string]any{"file": file}}
	default:
		return nil
	}
}
