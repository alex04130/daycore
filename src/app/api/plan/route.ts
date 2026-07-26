// Day plan CRUD
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { dayPlans, sessions } from "@/lib/db/schema";
import { eq, and, sql } from "drizzle-orm";

// GET /api/plan?sessionId=xxx&date=YYYY-MM-DD
export async function GET(req: NextRequest) {
  const { searchParams } = new URL(req.url);
  const sessionId = searchParams.get("sessionId");
  const date = searchParams.get("date");
  if (!sessionId || !date) return NextResponse.json({ error: "missing params" }, { status: 400 });

  try {
    const rows = await db
      .select()
      .from(dayPlans)
      .where(and(eq(dayPlans.sessionId, sessionId), eq(dayPlans.date, date)))
      .limit(1);
    return NextResponse.json(rows[0] ?? null);
  } catch (err) {
    console.error("[plan GET]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}

// POST /api/plan — create or update plan for date
export async function POST(req: NextRequest) {
  try {
    const { sessionId, date, blocks, note, sourceType } = await req.json();
    if (!sessionId || !date) return NextResponse.json({ error: "missing fields" }, { status: 400 });

    // Upsert
    const existing = await db
      .select({ id: dayPlans.id })
      .from(dayPlans)
      .where(and(eq(dayPlans.sessionId, sessionId), eq(dayPlans.date, date)))
      .limit(1);

    let plan;
    if (existing.length > 0) {
      [plan] = await db
        .update(dayPlans)
        .set({ blocks, note, sourceType, updatedAt: new Date() })
        .where(eq(dayPlans.id, existing[0].id))
        .returning();
    } else {
      [plan] = await db
        .insert(dayPlans)
        .values({ sessionId, date, blocks, note, sourceType: sourceType || "text" })
        .returning();

      // Increment interaction count
      await db.execute(
        sql`UPDATE sessions SET interaction_count = interaction_count + 1, updated_at = NOW() WHERE id = ${sessionId}`
      );
    }

    return NextResponse.json(plan);
  } catch (err) {
    console.error("[plan POST]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}

// PATCH /api/plan — apply incremental action (add/update/remove one block)
export async function PATCH(req: NextRequest) {
  try {
    const { sessionId, date, action } = await req.json();
    // action: { action: "update"|"remove"|"add", match?, changes?, block? }

    const rows = await db
      .select()
      .from(dayPlans)
      .where(and(eq(dayPlans.sessionId, sessionId), eq(dayPlans.date, date)))
      .limit(1);

    if (rows.length === 0) return NextResponse.json({ error: "plan_not_found" }, { status: 404 });

    const plan = rows[0];
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let blocks = (plan.blocks as any[]) || [];

    if (action.action === "update") {
      blocks = blocks.map((b) => {
        const matchKeys = Object.keys(action.match || {});
        const matched = matchKeys.every((k) => b[k] === action.match[k]);
        return matched ? { ...b, ...action.changes } : b;
      });
    } else if (action.action === "remove") {
      const matchKeys = Object.keys(action.match || {});
      blocks = blocks.filter((b) => {
        return !matchKeys.every((k) => b[k] === action.match[k]);
      });
    } else if (action.action === "add") {
      const newBlock = { ...action.block, id: `block-${Date.now()}` };
      blocks = [...blocks, newBlock].sort((a, b) => a.time.localeCompare(b.time));
    }

    const [updated] = await db
      .update(dayPlans)
      .set({ blocks, updatedAt: new Date() })
      .where(eq(dayPlans.id, plan.id))
      .returning();

    return NextResponse.json(updated);
  } catch (err) {
    console.error("[plan PATCH]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
