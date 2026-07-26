// Mood check-in CRUD
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { moodCheckins } from "@/lib/db/schema";
import { eq, desc, sql } from "drizzle-orm";

// GET /api/mood?sessionId=xxx&limit=10
export async function GET(req: NextRequest) {
  const { searchParams } = new URL(req.url);
  const sessionId = searchParams.get("sessionId");
  const limit = parseInt(searchParams.get("limit") || "10");
  if (!sessionId) return NextResponse.json({ error: "missing sessionId" }, { status: 400 });

  try {
    const rows = await db
      .select()
      .from(moodCheckins)
      .where(eq(moodCheckins.sessionId, sessionId))
      .orderBy(desc(moodCheckins.createdAt))
      .limit(limit);
    return NextResponse.json(rows);
  } catch (err) {
    console.error("[mood GET]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}

// POST /api/mood
export async function POST(req: NextRequest) {
  try {
    const { sessionId, mood, aiResponse, exerciseOffered, theme } = await req.json();
    if (!sessionId || !mood) return NextResponse.json({ error: "missing fields" }, { status: 400 });

    const [checkin] = await db
      .insert(moodCheckins)
      .values({ sessionId, mood, aiResponse, exerciseOffered, theme })
      .returning();

    // Increment interaction count
    await db.execute(
      sql`UPDATE sessions SET interaction_count = interaction_count + 1, updated_at = NOW() WHERE id = ${sessionId}`
    );

    return NextResponse.json(checkin);
  } catch (err) {
    console.error("[mood POST]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}

// PATCH /api/mood?id=xxx — mark exercise completed
export async function PATCH(req: NextRequest) {
  try {
    const { id } = await req.json();
    await db.update(moodCheckins).set({ exerciseCompleted: true }).where(eq(moodCheckins.id, id));
    return NextResponse.json({ ok: true });
  } catch (err) {
    console.error("[mood PATCH]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
