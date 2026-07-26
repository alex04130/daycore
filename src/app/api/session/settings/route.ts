// PATCH /api/session/settings — update assistant name and other per-session settings
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db/client";
import { sessions } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function PATCH(req: NextRequest) {
  try {
    const { sessionId, assistantName, currentTheme } = await req.json();
    if (!sessionId) return NextResponse.json({ error: "missing sessionId" }, { status: 400 });

    const updates: Record<string, unknown> = { updatedAt: new Date() };
    if (assistantName !== undefined) updates.assistantName = assistantName;
    if (currentTheme !== undefined) updates.currentTheme = currentTheme;

    const [updated] = await db
      .update(sessions)
      .set(updates)
      .where(eq(sessions.id, sessionId))
      .returning();

    return NextResponse.json(updated);
  } catch (err) {
    console.error("[session/settings]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
