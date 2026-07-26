package ai

// visionToolDefs are the function-calling tools handed to a text-only chat model
// so it can "see" an uploaded image indirectly: it asks read_image to OCR the
// whole picture, and zoom_image to magnify a region for small/blurry details.
// Each call is executed by routing the image to the separately-configured vision
// model (see vision.go).
func visionToolDefs() []ToolDef {
	return []ToolDef{
		{
			Name:        "read_image",
			Description: "读取用户上传的整张图片，提取其中的文字与日程/任务/时间/事件信息。需要了解图片内容时调用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"type":        "string",
						"description": "想从图片中了解的具体问题（可选）。",
					},
				},
			},
		},
		{
			Name:        "zoom_image",
			Description: "放大图片的某个矩形区域以看清细节（例如小字、表格某格）。坐标为相对比例 0~1。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x":        map[string]any{"type": "number", "description": "区域左上角 x，0~1"},
					"y":        map[string]any{"type": "number", "description": "区域左上角 y，0~1"},
					"width":    map[string]any{"type": "number", "description": "区域宽度占比，0~1"},
					"height":   map[string]any{"type": "number", "description": "区域高度占比，0~1"},
					"scale":    map[string]any{"type": "number", "description": "放大倍数，默认 2，最大 4"},
					"question": map[string]any{"type": "string", "description": "想看清的内容（可选）。"},
				},
				"required": []string{"x", "y", "width", "height"},
			},
		},
	}
}
