// POST /api/ai/plan-text
import { NextRequest, NextResponse } from "next/server";
import OpenAI from "openai";
import { PROMPTS } from "@/lib/ai/prompts";
import { buildDateContext } from "@/lib/date-context";

export async function POST(req: NextRequest) {
  const deepseek = new OpenAI({
    baseURL: "https://api.deepseek.com",
    apiKey: process.env.DEEPSEEK_API_KEY ?? "placeholder",
  });
  try {
    const { description, date, weekday, time, timezone, targetDate, targetWeekday, relativeDateMap } = await req.json();

    if (!description?.trim()) {
      return NextResponse.json({ error: "no_schedule_info", message: "请描述一下你今天有什么安排" });
    }

    // buildDateContext 用今天真实日期计算相对词；targetDate 是用户选定要规划的那天
    const ctx = buildDateContext(date, weekday, time, timezone);
    const systemPrompt = PROMPTS.dayPlanFromText({
      ...ctx,
      // 如果用户选了别的日期，覆盖掉默认值
      ...(targetDate ? { targetDate, targetWeekday: targetWeekday ?? ctx.weekday } : {}),
      // 客户端已经预算好的相对日期对照表优先
      relativeDateMap: relativeDateMap ?? ctx.relativeDateMap,
    });

    const response = await deepseek.chat.completions.create({
      model: "deepseek-chat",
      messages: [
        { role: "system", content: systemPrompt },
        { role: "user", content: description },
      ],
      temperature: 0.3,
      max_tokens: 2000,
      response_format: { type: "json_object" },
    });

    const raw = response.choices[0].message.content || "{}";
    let result: Record<string, unknown>;
    try {
      result = JSON.parse(raw);
    } catch {
      return NextResponse.json({ error: "parse_error", message: "日程解析出了点问题，请重试" });
    }

    if (result.error) return NextResponse.json(result);

    if (Array.isArray(result.blocks)) {
      result.blocks = result.blocks.map((b: Record<string, unknown>, i: number) => ({
        ...b,
        id: `block-${Date.now()}-${i}`,
        // 兜底：AI 如果漏了 date，强制补上 targetDate 或今天
        date: (b.date as string) || targetDate || date,
      }));
    }

    return NextResponse.json(result);
  } catch (err) {
    console.error("[plan-text]", err);
    return NextResponse.json({ error: "server_error", message: "日程生成出了点问题，请稍后再试" }, { status: 500 });
  }
}
