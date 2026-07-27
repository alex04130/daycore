package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
	"daycore/internal/i18n"
)

// categoryLine formats one material category for the inbox classifier prompt:
// id, display name, hint.
var categoryLine = i18n.Text{
	"zh-CN": "- %s（%s）：%s\n",
	"en-US": "- %s (%s): %s\n",
}

// inboxKey namespaces an uploaded file inside the session's temp-context store.
func inboxKey(tempID string) string { return "inbox:" + tempID }

// inboxDraftKey namespaces a classification draft awaiting user confirmation.
func inboxDraftKey(id string) string { return "inbox:draft:" + id }

// handleInboxProcess accepts a {text} or {temp_id} body and returns a
// structured analysis: AI classification (category + structured fields + a
// server-side draft for /api/inbox/commit) with the keyword categorizer as
// fallback when the AI is unavailable or unsure. A temp_id pulls back the
// previously uploaded content.
func (s *Server) handleInboxProcess(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Text   string `json:"text"`
		TempID string `json:"temp_id"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_body", "无法解析请求体")
		return
	}
	text := strings.TrimSpace(req.Text)

	// Pull an uploaded file back by temp_id when no inline text was given.
	if text == "" && req.TempID != "" {
		if tc, err := s.store.TempContexts().Get(r.Context(), sid, inboxKey(req.TempID)); err == nil {
			var meta struct {
				ContentType string `json:"contentType"`
				Data        string `json:"data"`
				Stored      bool   `json:"stored"`
				Filename    string `json:"filename"`
			}
			_ = json.Unmarshal([]byte(tc.Payload), &meta)
			switch {
			case meta.Stored && strings.HasPrefix(meta.ContentType, "text/"):
				if b, derr := base64.StdEncoding.DecodeString(meta.Data); derr == nil {
					text = strings.TrimSpace(string(b))
				}
			case strings.HasPrefix(meta.ContentType, "image/") && meta.Stored:
				// A photo: try food recognition first (diet capture), with the
				// timetable-extraction suggestion as fallback / alternative.
				s.processInboxImage(w, r, sid, meta.Data, meta.ContentType, meta.Filename)
				return
			case !strings.HasPrefix(meta.ContentType, "text/"):
				// Non-text, non-image (or an oversized image whose bytes weren't
				// kept): route to the vision endpoint rather than guessing here.
				s.writeJSON(w, http.StatusOK, map[string]any{
					"understanding": fmt.Sprintf("收到一个 %s 文件（%s）；图片请用 extract-schedule-image 识别。", meta.ContentType, meta.Filename),
					"suggestions":   []inboxSuggestion{{Action: "extract_image", Entity: "image", Summary: "用图片识别课表/日程"}},
				})
				return
			}
		}
	}
	if text == "" {
		s.writeErr(w, http.StatusBadRequest, "empty_text", "缺少 text 或有效的 temp_id")
		return
	}

	// AI classification; keyword categorizer as fallback (AI error, unparsable
	// output, or low confidence). The response keeps the legacy keys
	// {understanding, suggestions} so old clients keep working; new clients use
	// classification + draftId. The classifier call shares the AI rate budget
	// and timeout with the other one-shot AI endpoints.
	if !s.rateLimit(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()
	locale := s.requestLocale(r)
	cls, err := s.classifyInbox(ctx, sid, locale, text)
	if err != nil || cls.Confidence < 0.5 {
		if err != nil {
			s.log.Warn("inbox classify fallback", "err", err)
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"understanding": summarizeText(text),
			"suggestions":   s.categorizeText(text),
		})
		return
	}

	// Store the draft server-side (1h TTL) so commit executes the AI's
	// structured output without trusting a client round-trip.
	draftID := genTempID()
	payload, _ := json.Marshal(inboxDraft{Text: text, Classification: *cls})
	_ = s.store.TempContexts().Set(r.Context(), &domain.TempContext{
		SessionID: sid, Key: inboxDraftKey(draftID), Payload: string(payload), TTL: time.Now().Add(time.Hour),
	})

	s.writeJSON(w, http.StatusOK, map[string]any{
		"understanding":  cls.Summary,
		"suggestions":    suggestionsFromClassification(cls),
		"classification": cls,
		"draftId":        draftID,
	})
}

// ── AI classification ───────────────────────────────────────────────────────────

// inboxClassification is the classifier's parsed output; it rides the process
// response and the server-side draft verbatim.
type inboxClassification struct {
	Category        string         `json:"category"`
	Confidence      float64        `json:"confidence"`
	Title           string         `json:"title"`
	Summary         string         `json:"summary"`
	Structured      map[string]any `json:"structured,omitempty"`
	SuggestedAction map[string]any `json:"suggested_action,omitempty"`
	Advice          string         `json:"advice,omitempty"`
}

// inboxDraft is what /api/inbox/commit executes after user confirmation.
type inboxDraft struct {
	Text           string              `json:"text"`
	Classification inboxClassification `json:"classification"`
}

// classifyInbox runs the one-shot inbox_classify prompt over the session's
// enabled categories. Unknown or disabled categories collapse to note.
func (s *Server) classifyInbox(ctx context.Context, sid, locale, text string) (*inboxClassification, error) {
	enabled := s.enabledMaterialCategories(ctx, sid)
	var list strings.Builder
	// The punctuation is part of the translation: full-width （）： belongs in a
	// Chinese prompt and reads as a typo in an English one. Verbs are id, name,
	// hint — in that order, in every locale.
	for _, c := range domain.MaterialCategories() {
		if !enabled[c.ID] {
			continue
		}
		fmt.Fprintf(&list, i18n.Pick(categoryLine, locale), c.ID, c.Name(locale), c.Hint(locale))
	}
	prompt, err := s.prompts.Render(ctx, ai.PromptInboxClassify, locale, ai.InboxClassifyData{
		Text: text, Categories: list.String(), Date: time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleUser, Content: prompt}},
		JSONMode:    true,
		Temperature: 0.2,
		MaxTokens:   1024,
	})
	if err != nil {
		return nil, err
	}
	obj, ok := extractJSONObject(resp.Content)
	if !ok {
		return nil, fmt.Errorf("inbox classify: no JSON object in model output")
	}
	b, _ := json.Marshal(obj)
	var out inboxClassification
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if _, known := domain.MaterialCategoryByID(out.Category); !known || !enabled[out.Category] {
		out.Category = domain.CategoryNote
	}
	return &out, nil
}

// processInboxImage classifies an uploaded photo. With the diet category
// enabled it runs food recognition through the vision pipeline and returns a
// diet confirmation card (same classification + draftId shape as text). A
// non-food image, disabled diet, or any vision failure falls back to the
// timetable-extraction suggestion.
func (s *Server) processInboxImage(w http.ResponseWriter, r *http.Request, sid, imageB64, contentType, filename string) {
	fallback := func() {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"understanding": fmt.Sprintf("收到一个 %s 文件（%s）;图片请用 extract-schedule-image 识别。", contentType, filename),
			"suggestions":   []inboxSuggestion{{Action: "extract_image", Entity: "image", Summary: "用图片识别课表/日程"}},
		})
	}
	if !s.enabledMaterialCategories(r.Context(), sid)["diet"] {
		fallback()
		return
	}
	if !s.rateLimit(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()
	locale := s.requestLocale(r)
	prompt, err := s.prompts.Render(ctx, ai.PromptFoodRecognize, locale, ai.FoodRecognizeData{})
	if err != nil {
		fallback()
		return
	}
	raw, err := s.vision.PlanFromImage(ctx, s.catalog.DefaultChat(), prompt, imageB64, contentType)
	if err != nil {
		s.log.Warn("inbox food recognize fallback", "err", err)
		fallback()
		return
	}
	obj, ok := extractJSONObject(raw)
	if !ok {
		fallback()
		return
	}
	isFood, _ := obj["is_food"].(bool)
	confidence, _ := obj["confidence"].(float64)
	if !isFood || confidence < 0.5 {
		fallback()
		return
	}
	title, _ := obj["title"].(string)
	summary, _ := obj["summary"].(string)
	structured := map[string]any{}
	for _, k := range []string{"items", "total", "meal"} {
		if v, exists := obj[k]; exists {
			structured[k] = v
		}
	}
	cls := &inboxClassification{
		Category: "diet", Confidence: confidence, Title: title, Summary: summary,
		Structured:      structured,
		SuggestedAction: map[string]any{"action": "save_material", "entity": "diet"},
	}
	draftID := genTempID()
	payload, _ := json.Marshal(inboxDraft{Text: "[photo] " + filename, Classification: *cls})
	_ = s.store.TempContexts().Set(r.Context(), &domain.TempContext{
		SessionID: sid, Key: inboxDraftKey(draftID), Payload: string(payload), TTL: time.Now().Add(time.Hour),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{
		"understanding": summary,
		// The photo might still be a timetable — keep that path one tap away.
		"suggestions": []inboxSuggestion{
			{Action: "save_material", Entity: "diet", Summary: title},
			{Action: "extract_image", Entity: "image", Summary: "这其实是课表/日程截图"},
		},
		"classification": cls,
		"draftId":        draftID,
	})
}

// suggestionsFromClassification keeps the legacy suggestions shape populated
// from the AI result so pre-classifier clients render something sensible.
func suggestionsFromClassification(cls *inboxClassification) []inboxSuggestion {
	action, entity := "save_material", cls.Category
	if a, ok := cls.SuggestedAction["action"].(string); ok && a != "" {
		action = a
	}
	if e, ok := cls.SuggestedAction["entity"].(string); ok && e != "" {
		entity = e
	}
	summary := cls.Summary
	if summary == "" {
		summary = cls.Title
	}
	return []inboxSuggestion{{Action: action, Entity: entity, Summary: summary}}
}

// POST /api/inbox/commit — execute a confirmed classification draft. The
// mapping from classification to module lives here once, server-side, so web /
// native / channel clients don't each reimplement it (and the structured JSON
// never round-trips through the client). Clients may still call the module
// endpoints directly — commit is the recommended path, not the only one.
func (s *Server) handleInboxCommit(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var req struct {
		DraftID   string `json:"draftId"`
		Overrides struct {
			Category *string `json:"category"`
			Title    *string `json:"title"`
			Summary  *string `json:"summary"`
			Body     *string `json:"body"`
		} `json:"overrides"`
		// AlsoKeepNote keeps the raw capture as a material even when the draft
		// routes to another module (manual assignment).
		AlsoKeepNote bool `json:"alsoKeepNote"`
	}
	if err := s.readJSON(r, &req); err != nil || req.DraftID == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 draftId")
		return
	}
	tc, err := s.store.TempContexts().Get(r.Context(), sid, inboxDraftKey(req.DraftID))
	if err != nil || tc == nil {
		s.writeErr(w, http.StatusNotFound, "draft_not_found", "草稿不存在或已过期")
		return
	}
	var draft inboxDraft
	if err := json.Unmarshal([]byte(tc.Payload), &draft); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "草稿数据损坏")
		return
	}
	cls := draft.Classification
	if req.Overrides.Category != nil {
		cls.Category = *req.Overrides.Category
	}
	if req.Overrides.Title != nil {
		cls.Title = *req.Overrides.Title
	}
	if req.Overrides.Summary != nil {
		cls.Summary = *req.Overrides.Summary
	}
	enabled := s.enabledMaterialCategories(r.Context(), sid)
	if _, known := domain.MaterialCategoryByID(cls.Category); !known || !enabled[cls.Category] {
		s.writeErr(w, http.StatusBadRequest, "bad_category", "类别未启用或不存在: "+cls.Category)
		return
	}

	// Academic captures whose classification carries a deadline become manual
	// assignments — the module auto-plan already consumes — instead of (or in
	// addition to, with alsoKeepNote) a material.
	var assignment *domain.Assignment
	if cls.Category == "academic" {
		if action, _ := cls.SuggestedAction["action"].(string); action == "create_assignment" {
			title := strings.TrimSpace(cls.Title)
			var dueAt *time.Time
			if st := cls.Structured; st != nil {
				if v, ok := st["title"].(string); ok && strings.TrimSpace(v) != "" {
					title = strings.TrimSpace(v)
				}
				if v, ok := st["due_at"].(string); ok && v != "" {
					if t, err := parseFlexibleTime(v); err == nil {
						dueAt = &t
					}
				}
			}
			if title == "" {
				title = truncate(draft.Text, 20)
			}
			a, err := s.createManualAssignment(r.Context(), sid, title, "", dueAt)
			if err != nil {
				s.writeErr(w, http.StatusInternalServerError, "internal", "作业创建失败")
				return
			}
			assignment = a
			if !req.AlsoKeepNote {
				s.writeJSON(w, http.StatusOK, map[string]any{"type": "assignment", "assignment": assignment})
				return
			}
		}
	}

	body := ""
	if req.Overrides.Body != nil {
		body = *req.Overrides.Body
	} else {
		payload := map[string]any{"text": draft.Text}
		if len(cls.Structured) > 0 {
			payload["structured"] = cls.Structured
		}
		if cls.Advice != "" {
			payload["advice"] = cls.Advice
		}
		b, _ := json.Marshal(payload)
		body = string(b)
	}
	title := strings.TrimSpace(cls.Title)
	if title == "" {
		title = truncate(draft.Text, 20)
	}
	created, err := s.store.Materials().Create(r.Context(), &domain.Material{
		SessionID: sid, Category: cls.Category, Title: title, Summary: cls.Summary,
		Body: body, Source: "inbox", MimeType: "application/json",
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "素材创建失败")
		return
	}
	resp := map[string]any{"type": "material", "material": created}
	if assignment != nil {
		resp["type"] = "assignment"
		resp["assignment"] = assignment
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleInboxUpload accepts a file upload, persists it (small files) so /process
// can consume it, and returns a temporary ID.
func (s *Server) handleInboxUpload(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.authRateLimit(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20) // 8 MB cap
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_upload", "上传失败，请检查文件大小或格式")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "missing_file", "缺少 file 字段")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "read_error", "读取文件失败")
		return
	}

	tempID := genTempID()
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = detectContentType(header.Filename, raw)
	}

	// Persist small files so /process can consume them (1h TTL); large files keep
	// the temp_id + metadata but no bytes.
	const maxStored = 1 << 20 // 1 MB
	stored := len(raw) <= maxStored
	meta := map[string]any{"filename": header.Filename, "contentType": contentType, "size": len(raw), "stored": stored}
	if stored {
		meta["data"] = base64.StdEncoding.EncodeToString(raw)
	}
	payload, _ := json.Marshal(meta)
	_ = s.store.TempContexts().Set(r.Context(), &domain.TempContext{
		SessionID: sid, Key: inboxKey(tempID), Payload: string(payload), TTL: time.Now().Add(time.Hour),
	})

	s.writeJSON(w, http.StatusOK, map[string]any{
		"temp_id":      tempID,
		"filename":     header.Filename,
		"size":         len(raw),
		"content_type": contentType,
		"stored":       stored,
	})
}

// ── rule-based text categorizer (placeholder AI logic) ──────────────────────────

type inboxSuggestion struct {
	Action  string `json:"action"`
	Entity  string `json:"entity"`
	Summary string `json:"summary"`
}

func (s *Server) categorizeText(text string) []inboxSuggestion {
	lower := strings.ToLower(text)
	var out []inboxSuggestion

	switch {
	case containsAny(lower, "计划", "规划", "安排", "plan", "schedule", "task"):
		out = append(out, inboxSuggestion{
			Action:  "create_plan",
			Entity:  "plan",
			Summary: "将内容转为日程计划",
		})
	case containsAny(lower, "心情", "情绪", "mood", "feeling"):
		out = append(out, inboxSuggestion{
			Action:  "log_mood",
			Entity:  "mood",
			Summary: "记录一条心情",
		})
	case containsAny(lower, "提醒", "remind", "alarm"):
		out = append(out, inboxSuggestion{
			Action:  "create_rule",
			Entity:  "rule",
			Summary: "创建提醒规则",
		})
	case containsAny(lower, "记忆", "记住", "memor", "memory"):
		out = append(out, inboxSuggestion{
			Action:  "save_memory",
			Entity:  "memory",
			Summary: "保存为长期记忆",
		})
	}

	// Always include a generic "keep in inbox" fallback.
	if len(out) == 0 {
		out = append(out, inboxSuggestion{
			Action:  "keep",
			Entity:  "note",
			Summary: "暂存为普通笔记，稍后处理",
		})
	}
	return out
}

func summarizeText(text string) string {
	runes := []rune(text)
	if len(runes) <= 80 {
		return fmt.Sprintf("收到一段短文本（%d 字）:%s", len(runes), truncate(text, 40))
	}
	return fmt.Sprintf("收到一段较长的文本（%d 字）:%s…", len(runes), truncate(text, 30))
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// ── upload helpers ──────────────────────────────────────────────────────────────

func genTempID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

func detectContentType(filename string, raw []byte) string {
	if len(raw) >= 4 {
		// PNG magic
		if raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47 {
			return "image/png"
		}
		// JPEG magic
		if raw[0] == 0xFF && raw[1] == 0xD8 {
			return "image/jpeg"
		}
		// GIF magic
		if raw[0] == 0x47 && raw[1] == 0x49 && raw[2] == 0x46 {
			return "image/gif"
		}
		// PDF magic
		if raw[0] == 0x25 && raw[1] == 0x50 && raw[2] == 0x44 && raw[3] == 0x46 {
			return "application/pdf"
		}
	}
	// Heuristic by extension.
	if strings.HasSuffix(strings.ToLower(filename), ".txt") {
		return "text/plain"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".md") {
		return "text/markdown"
	}
	return "application/octet-stream"
}
