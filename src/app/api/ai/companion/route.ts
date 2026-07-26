// POST /api/ai/companion
// Companion chat using DeepSeek v4-pro with streaming SSE
import { NextRequest, NextResponse } from "next/server";
import OpenAI from "openai";
import { PROMPTS } from "@/lib/ai/prompts";

export async function POST(req: NextRequest) {
  const deepseek = new OpenAI({
    baseURL: "https://api.deepseek.com",
    apiKey: process.env.DEEPSEEK_API_KEY ?? "placeholder",
  });
  try {
    const {
      message,
      date, weekday, time, timezone,
      theme, todayPlan, moodHistory, memoryContext, assistantName,
      conversationHistory = [],
    } = await req.json();

    const systemPrompt = PROMPTS.companion({
      date, weekday, time, timezone,
      theme,
      todayPlan: JSON.stringify(todayPlan || []),
      moodHistory: JSON.stringify(moodHistory || []),
      memoryContext: JSON.stringify(memoryContext || []),
      assistantName: assistantName || "Leo",
    });

    const messages: OpenAI.Chat.ChatCompletionMessageParam[] = [
      { role: "system", content: systemPrompt },
      ...conversationHistory.slice(-20), // last 20 messages for context
      { role: "user", content: message },
    ];

    const stream = await deepseek.chat.completions.create({
      model: "deepseek-chat",
      messages,
      stream: true,
      temperature: 0.7,
      max_tokens: 800,
    });

    const encoder = new TextEncoder();
    const readable = new ReadableStream({
      async start(controller) {
        for await (const chunk of stream) {
          const delta = chunk.choices[0]?.delta?.content || "";
          if (delta) {
            controller.enqueue(encoder.encode(`data: ${JSON.stringify({ delta })}\n\n`));
          }
        }
        controller.enqueue(encoder.encode("data: [DONE]\n\n"));
        controller.close();
      },
    });

    return new NextResponse(readable, {
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
      },
    });
  } catch (err) {
    console.error("[companion]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
