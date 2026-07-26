// GET/POST /api/companion-history — persist conversation per session
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { companionMemory } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function GET(req: NextRequest) {
  const sessionId = new URL(req.url).searchParams.get("sessionId");
  if (!sessionId) return NextResponse.json({ history: [], keyFacts: [] });

  try {
    const rows = await db.select().from(companionMemory)
      .where(eq(companionMemory.sessionId, sessionId)).limit(1);
    if (!rows.length) return NextResponse.json({ history: [], keyFacts: [] });
    return NextResponse.json({
      history: rows[0].conversationHistory ?? [],
      keyFacts: rows[0].keyFacts ?? [],
    });
  } catch {
    return NextResponse.json({ history: [], keyFacts: [] });
  }
}

export async function POST(req: NextRequest) {
  try {
    const { sessionId, history, keyFacts } = await req.json();
    if (!sessionId) return NextResponse.json({ ok: false }, { status: 400 });

    const existing = await db.select({ id: companionMemory.id })
      .from(companionMemory).where(eq(companionMemory.sessionId, sessionId)).limit(1);

    if (existing.length) {
      await db.update(companionMemory)
        .set({ conversationHistory: history, keyFacts: keyFacts ?? [], updatedAt: new Date() })
        .where(eq(companionMemory.id, existing[0].id));
    } else {
      await db.insert(companionMemory)
        .values({ sessionId, conversationHistory: history, keyFacts: keyFacts ?? [] });
    }
    return NextResponse.json({ ok: true });
  } catch (err) {
    console.error("[companion-history POST]", err);
    return NextResponse.json({ ok: false }, { status: 500 });
  }
}
