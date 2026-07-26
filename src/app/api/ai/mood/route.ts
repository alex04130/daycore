// POST /api/ai/mood
// Mood response using DeepSeek v4-pro
import { NextRequest, NextResponse } from "next/server";
import OpenAI from "openai";
import { PROMPTS } from "@/lib/ai/prompts";

export async function POST(req: NextRequest) {
  const deepseek = new OpenAI({
    baseURL: "https://api.deepseek.com",
    apiKey: process.env.DEEPSEEK_API_KEY ?? "placeholder",
  });
  try {
    const { mood } = await req.json();
    const prompt = PROMPTS.moodResponse(mood);

    const response = await deepseek.chat.completions.create({
      model: "deepseek-chat",
      messages: [{ role: "user", content: prompt }],
      temperature: 0.7,
      max_tokens: 200,
    });

    const text = response.choices[0].message.content || "";
    return NextResponse.json({ response: text });
  } catch (err) {
    console.error("[mood]", err);
    return NextResponse.json(
      { error: "server_error", response: "遇到了点问题，稍后再试一下吧。" },
      { status: 500 }
    );
  }
}
