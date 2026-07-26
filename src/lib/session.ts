// Session management — anonymous-first with optional sign-in
// Session ID stored in localStorage, data persisted in DB by session ID

import { db } from "@/lib/db/client";
import { sessions } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function getOrCreateSession(sessionId: string) {
  const existing = await db
    .select()
    .from(sessions)
    .where(eq(sessions.id, sessionId))
    .limit(1);

  if (existing.length > 0) return existing[0];

  const [created] = await db
    .insert(sessions)
    .values({ id: sessionId })
    .returning();

  return created;
}

export async function incrementInteractionCount(sessionId: string) {
  const [updated] = await db
    .update(sessions)
    .set({
      interactionCount: db.$count(sessions), // placeholder — use raw SQL below
      updatedAt: new Date(),
    })
    .where(eq(sessions.id, sessionId))
    .returning();
  return updated;
}

export async function markSignInPrompted(sessionId: string) {
  await db
    .update(sessions)
    .set({ signInPrompted: true, updatedAt: new Date() })
    .where(eq(sessions.id, sessionId));
}

export async function getSessionSettings(sessionId: string) {
  const rows = await db
    .select({ assistantName: sessions.assistantName, currentTheme: sessions.currentTheme, interactionCount: sessions.interactionCount, signInPrompted: sessions.signInPrompted })
    .from(sessions)
    .where(eq(sessions.id, sessionId))
    .limit(1);
  return rows[0] ?? { assistantName: "Leo", currentTheme: "sky", interactionCount: 0, signInPrompted: false };
}

export async function updateSessionSettings(sessionId: string, settings: { assistantName?: string; currentTheme?: string }) {
  await db
    .update(sessions)
    .set({ ...settings, updatedAt: new Date() })
    .where(eq(sessions.id, sessionId));
}
