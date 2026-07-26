// POST /api/session/init — get or create anonymous session
import { NextRequest, NextResponse } from "next/server";
import { getOrCreateSession } from "@/lib/session";

export async function POST(req: NextRequest) {
  try {
    const { sessionId } = await req.json();
    if (!sessionId) return NextResponse.json({ error: "missing sessionId" }, { status: 400 });
    const session = await getOrCreateSession(sessionId);
    return NextResponse.json(session);
  } catch (err) {
    console.error("[session/init]", err);
    return NextResponse.json({ error: "server_error" }, { status: 500 });
  }
}
