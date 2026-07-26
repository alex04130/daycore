// GET /api/plan/range?sessionId=xxx&from=YYYY-MM-DD&to=YYYY-MM-DD
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { dayPlans } from "@/lib/db/schema";
import { eq, and, gte, lte } from "drizzle-orm";

export async function GET(req: NextRequest) {
  const { searchParams } = new URL(req.url);
  const sessionId = searchParams.get("sessionId");
  const from = searchParams.get("from");
  const to = searchParams.get("to");

  if (!sessionId || !from || !to) {
    return NextResponse.json({ error: "missing params" }, { status: 400 });
  }

  try {
    const rows = await db
      .select()
      .from(dayPlans)
      .where(
        and(
          eq(dayPlans.sessionId, sessionId),
          gte(dayPlans.date, from),
          lte(dayPlans.date, to)
        )
      )
      .orderBy(dayPlans.date);

    return NextResponse.json(rows);
  } catch (err) {
    console.error("[plan/range GET]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
