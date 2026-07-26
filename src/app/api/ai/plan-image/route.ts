// POST /api/ai/plan-image
// Generate day plan from uploaded image using Eazo Built-in AI (Gemini vision)
import { NextRequest, NextResponse } from "next/server";
import { ai } from "@eazo/sdk";
import { PROMPTS } from "@/lib/ai/prompts";
import { buildDateContext } from "@/lib/date-context";

export async function POST(req: NextRequest) {
  try {
    const { imageBase64, mimeType, date, weekday, time, timezone, targetDate, targetWeekday, relativeDateMap } = await req.json();

    if (!imageBase64) {
      return NextResponse.json({ error: "no_image", message: "请上传图片" });
    }

    const ctx = buildDateContext(date, weekday, time, timezone);
    const systemPrompt = PROMPTS.dayPlanFromImage({
      ...ctx,
      ...(targetDate ? { targetDate, targetWeekday: targetWeekday ?? ctx.weekday } : {}),
      relativeDateMap: relativeDateMap ?? ctx.relativeDateMap,
    });

    const response = await ai.chat({
      model: "google.gemma-3-27b-it", // vision-capable model in Eazo built-in AI
      messages: [
        { role: "system", content: systemPrompt },
        {
          role: "user",
          content: [
            {
              type: "image_url",
              image_url: {
                url: `data:${mimeType || "image/jpeg"};base64,${imageBase64}`,
              },
            },
            {
              type: "text",
              text: "请读取这张图片，提取日程信息并按格式返回 JSON。",
            },
          ],
        },
      ],
      response_format: { type: "json_object" },
      temperature: 0.2,
    });

    const content = response.choices[0].message.content || "{}";
    const result = JSON.parse(content);

    if (result.blocks) {
      result.blocks = result.blocks.map((b: Record<string, unknown>, i: number) => ({
        ...b,
        id: `block-${Date.now()}-${i}`,
      }));
    }

    return NextResponse.json(result);
  } catch (err) {
    console.error("[plan-image]", err);
    return NextResponse.json(
      { error: "server_error", message: "图片读取出了点问题，请稍后再试" },
      { status: 500 }
    );
  }
}
