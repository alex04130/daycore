package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoVisionModel is returned when image input is requested but no vision-capable
// model is configured and the chat model itself cannot see.
var ErrNoVisionModel = errors.New("no vision-capable model configured (set DEFAULT_VISION_MODEL or mark a catalog model vision: true)")

const maxVisionToolRounds = 6

// Orchestrator turns an uploaded image into a model response, choosing the best
// strategy based on model capabilities:
//   - chat model has native vision        → send the image directly
//   - chat model has tools (no vision)    → function-calling: it drives read_image
//     / zoom_image, each routed to the separate vision model
//   - chat model has neither              → fall back to the vision model directly
type Orchestrator struct {
	catalog *Catalog
}

// NewOrchestrator builds a vision Orchestrator over the model catalog.
func NewOrchestrator(c *Catalog) *Orchestrator { return &Orchestrator{catalog: c} }

// PlanFromImage runs systemPrompt against the image and returns the model's text
// (expected to be JSON for the planning flow).
func (o *Orchestrator) PlanFromImage(ctx context.Context, chat AIProvider, systemPrompt, imageB64, mime string) (string, error) {
	caps := chat.Capabilities()

	// 1) Native vision: hand the image straight to the chat model.
	if caps.Vision {
		return directVision(ctx, chat, systemPrompt, imageB64, mime, true)
	}

	vision, hasVision := o.catalog.Vision()
	if !hasVision {
		return "", ErrNoVisionModel
	}

	// 2) No vision but no tool support either: let the vision model do it all.
	if !caps.Tools {
		return directVision(ctx, vision, systemPrompt, imageB64, mime, true)
	}

	// 3) Tool-calling: the text model inspects the image via read_image/zoom_image,
	//    each executed against the vision model, then emits the final JSON.
	messages := []Message{
		{Role: RoleSystem, Content: systemPrompt + "\n\n注意：用户上传了一张图片，但你无法直接看到它。请调用 read_image 读取整张图片，必要时用 zoom_image 放大局部看清细节。获得足够信息后，按系统提示要求直接输出最终 JSON（届时不要再调用工具）。"},
		{Role: RoleUser, Content: "我上传了一张图片，请帮我把它整理成日程。"},
	}
	tools := visionToolDefs()

	for round := 0; round < maxVisionToolRounds; round++ {
		resp, err := chat.Chat(ctx, ChatRequest{
			Messages:    messages,
			Tools:       tools,
			Temperature: 0.2,
			MaxTokens:   2000,
		})
		if err != nil {
			return "", err
		}
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}
		messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			result, err := o.runVisionTool(ctx, vision, call.Name, call.Arguments, imageB64, mime)
			if err != nil {
				result = "工具执行失败：" + err.Error()
			}
			messages = append(messages, Message{Role: RoleTool, ToolCallID: call.ID, Name: call.Name, Content: result})
		}
	}

	// Out of rounds: force a final answer without tools.
	messages = append(messages, Message{Role: RoleUser, Content: "请根据以上读取到的信息，现在直接输出最终 JSON，不要再调用工具。"})
	resp, err := chat.Chat(ctx, ChatRequest{Messages: messages, Temperature: 0.2, MaxTokens: 2000, JSONMode: true})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func directVision(ctx context.Context, p AIProvider, systemPrompt, imageB64, mime string, jsonMode bool) (string, error) {
	resp, err := p.Chat(ctx, ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: systemPrompt},
			{Role: RoleUser, Parts: []ContentPart{
				{Type: PartText, Text: "请读取这张图片，提取日程信息并按格式返回 JSON。"},
				{Type: PartImage, Data: imageB64, MIME: mime},
			}},
		},
		JSONMode:    jsonMode,
		Temperature: 0.2,
		MaxTokens:   2000,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (o *Orchestrator) runVisionTool(ctx context.Context, vision AIProvider, name, argsJSON, imageB64, mime string) (string, error) {
	switch name {
	case "read_image":
		var a struct {
			Question string `json:"question"`
		}
		_ = json.Unmarshal([]byte(argsJSON), &a)
		q := a.Question
		if q == "" {
			q = "请仔细阅读这张图片，提取其中所有日程、任务、时间、事件信息，逐条原样转述，不要遗漏也不要编造。"
		}
		return askVision(ctx, vision, q, imageB64, mime)

	case "zoom_image":
		var a struct {
			X        float64 `json:"x"`
			Y        float64 `json:"y"`
			Width    float64 `json:"width"`
			Height   float64 `json:"height"`
			Scale    float64 `json:"scale"`
			Question string  `json:"question"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "", fmt.Errorf("bad zoom_image args: %w", err)
		}
		img, err := decodeImage(imageB64)
		if err != nil {
			return "", err
		}
		scale := a.Scale
		if scale == 0 {
			scale = 2
		}
		zoomed := cropAndScale(img, a.X, a.Y, a.Width, a.Height, scale)
		zb64, err := encodePNG(zoomed)
		if err != nil {
			return "", err
		}
		q := a.Question
		if q == "" {
			q = "这是放大后的图片区域，请仔细识别其中的文字和细节，逐条转述。"
		}
		return askVision(ctx, vision, q, zb64, "image/png")

	default:
		return "", fmt.Errorf("unknown vision tool %q", name)
	}
}

func askVision(ctx context.Context, vision AIProvider, question, b64, mime string) (string, error) {
	resp, err := vision.Chat(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Parts: []ContentPart{
			{Type: PartText, Text: question},
			{Type: PartImage, Data: b64, MIME: mime},
		}}},
		Temperature: 0.2,
		MaxTokens:   1200,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
