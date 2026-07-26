package onebot

import (
	"context"
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

func (a *Adapter) Name() string { return "onebot" }

func (a *Adapter) Start(ctx context.Context, inbound chan<- channels.InboundMsg) error {
	a.inbound = inbound
	a.stop = make(chan struct{})
	a.reconnect = 1 * time.Second
	go a.loop(ctx)
	return nil
}

func (a *Adapter) Stop() error { close(a.stop); return nil }

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
		_ = a.Send(ctx, externalID, "消息太频繁，请稍后再试")
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

func (a *Adapter) Send(ctx context.Context, externalID string, msg string) error {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	payload := map[string]any{
		"action": "send_private_msg",
		"params": map[string]any{
			"user_id": externalID,
			"message": msg,
		},
	}
	return conn.WriteJSON(payload)
}
