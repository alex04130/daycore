// POST /api/session/theme — log theme switch
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { themeSwitchLog, sessions } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function POST(req: NextRequest) {
  try {
    const { sessionId, theme } = await req.json();
    if (!sessionId || !theme) return NextResponse.json({ error: "missing fields" }, { status: 400 });

    await Promise.all([
      db.insert(themeSwitchLog).values({ sessionId, theme }),
      db.update(sessions).set({ currentTheme: theme, updatedAt: new Date() }).where(eq(sessions.id, sessionId)),
    ]);

    return NextResponse.json({ ok: true });
  } catch (err) {
    console.error("[session/theme]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
