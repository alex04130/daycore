package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"daycore/internal/ai"
)

// POST /api/ai/plan-text — natural language → structured day plan (JSON).
func (s *Server) handleAIPlanText(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Description   string `json:"description"`
		Date          string `json:"date"`
		Weekday       string `json:"weekday"`
		Time          string `json:"time"`
		Timezone      string `json:"timezone"`
		TargetDate    string `json:"targetDate"`
		TargetWeekday string `json:"targetWeekday"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if strings.TrimSpace(body.Description) == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "no_schedule_info", "message": "请描述一下你今天有什么安排"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	dc := ai.BuildDateContext(body.Date, body.Weekday, body.Time, body.Timezone, s.requestLocale(r))
	sys, err := s.prompts.Render(ctx, ai.PromptDayPlanText, s.requestLocale(r), ai.PlanTextData{
		Date: dc.Date, Weekday: dc.Weekday, Time: dc.Time, Timezone: dc.Timezone,
		TargetDate: body.TargetDate, TargetWeekday: orDefault(body.TargetWeekday, dc.Weekday),
		RelativeDateMap: dc.RelativeDateMap,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "提示词渲染失败")
		return
	}

	resp, err := s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sys},
			{Role: ai.RoleUser, Content: body.Description},
		},
		Temperature: 0.3, MaxTokens: 4096, JSONMode: true,
	})
	if err != nil {
		s.log.Error("ai plan-text", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "message": "日程生成出了点问题，请稍后再试"})
		return
	}

	result, ok := extractJSONObject(resp.Content)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "日程解析出了点问题，请重试"})
		return
	}
	if result["error"] != nil {
		s.writeJSON(w, http.StatusOK, result)
		return
	}
	attachBlocks(result, orDefault(body.TargetDate, body.Date))
	s.writeJSON(w, http.StatusOK, result)
}

// POST /api/ai/plan-image — image → structured day plan via the vision pipeline.
func (s *Server) handleAIPlanImage(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		ImageBase64 string `json:"imageBase64"`
		MimeType    string `json:"mimeType"`
		Date        string `json:"date"`
		Weekday     string `json:"weekday"`
		Time        string `json:"time"`
		Timezone    string `json:"timezone"`
		TargetDate  string `json:"targetDate"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if body.ImageBase64 == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "no_image", "message": "请上传图片"})
		return
	}
	if int64(len(body.ImageBase64))*3/4 > s.cfg.MaxImageBytes {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "image_too_large", "message": "图片太大了，换一张小一点的截图试试？"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	dc := ai.BuildDateContext(body.Date, body.Weekday, body.Time, body.Timezone, s.requestLocale(r))
	sys, err := s.prompts.Render(ctx, ai.PromptDayPlanImage, s.requestLocale(r), ai.PlanImageData{
		Date: dc.Date, Weekday: dc.Weekday, Time: dc.Time, Timezone: dc.Timezone,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "提示词渲染失败")
		return
	}

	mime := body.MimeType
	if mime == "" {
		mime = "image/jpeg"
	}
	content, err := s.vision.PlanFromImage(ctx, s.catalog.DefaultChat(), sys, body.ImageBase64, mime)
	if errors.Is(err, ai.ErrNoVisionModel) {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "vision_unavailable", "message": "当前没有配置可读图的视觉模型，先用文字描述安排吧"})
		return
	}
	if err != nil {
		s.log.Error("ai plan-image", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "message": "图片读取出了点问题，请稍后再试"})
		return
	}

	result, ok := extractJSONObject(content)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "图片解析出了点问题，请重试"})
		return
	}
	if result["error"] != nil {
		s.writeJSON(w, http.StatusOK, result)
		return
	}
	attachBlocks(result, orDefault(body.TargetDate, body.Date))
	s.writeJSON(w, http.StatusOK, result)
}

// POST /api/ai/extract-schedule-image — weekly timetable screenshot → recurring
// rule candidates (returned for user confirmation; nothing is saved here — the
// frontend saves confirmed rules via POST /api/rules/batch).
func (s *Server) handleAIExtractScheduleImage(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		ImageBase64 string `json:"imageBase64"`
		MimeType    string `json:"mimeType"`
		Date        string `json:"date"`
		Weekday     string `json:"weekday"`
		Time        string `json:"time"`
		Timezone    string `json:"timezone"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if body.ImageBase64 == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "no_image", "message": "请上传图片"})
		return
	}
	if int64(len(body.ImageBase64))*3/4 > s.cfg.MaxImageBytes {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "image_too_large", "message": "图片太大了，换一张小一点的截图试试？"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	dc := ai.BuildDateContext(body.Date, body.Weekday, body.Time, body.Timezone, s.requestLocale(r))
	sys, err := s.prompts.Render(ctx, ai.PromptScheduleExtractImage, s.requestLocale(r), ai.PlanImageData{
		Date: dc.Date, Weekday: dc.Weekday, Time: dc.Time, Timezone: dc.Timezone,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "提示词渲染失败")
		return
	}

	mime := body.MimeType
	if mime == "" {
		mime = "image/jpeg"
	}
	content, err := s.vision.PlanFromImage(ctx, s.catalog.DefaultChat(), sys, body.ImageBase64, mime)
	if errors.Is(err, ai.ErrNoVisionModel) {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "vision_unavailable", "message": "当前没有配置可读图的视觉模型"})
		return
	}
	if err != nil {
		s.log.Error("ai extract-schedule-image", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "message": "图片读取出了点问题，请稍后再试"})
		return
	}

	result, ok := extractJSONObject(content)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "图片解析出了点问题，请重试"})
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
