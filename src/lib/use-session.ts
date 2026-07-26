"use client";

import { useEffect, useState } from "react";
import { v4 as uuid } from "uuid";

interface Session {
  sessionId: string;
  interactionCount: number;
  signInPrompted: boolean;
  assistantName: string;
  currentTheme: string;
}

// Custom event name for same-page broadcasts
const ASSISTANT_NAME_EVENT = "daycore:assistant-name-changed";

export function broadcastAssistantName(name: string) {
  localStorage.setItem("daycore-assistant-name", name);
  window.dispatchEvent(new CustomEvent(ASSISTANT_NAME_EVENT, { detail: name }));
}

export function useSession() {
  const [session, setSession] = useState<Session | null>(null);

  useEffect(() => {
    let sid = localStorage.getItem("daycore-session-id");
    if (!sid) {
      sid = uuid();
      localStorage.setItem("daycore-session-id", sid);
    }

    fetch("/api/session/init", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId: sid }),
    })
      .then((res) => res.json())
      .then((data) => {
        const localName = localStorage.getItem("daycore-assistant-name");
        setSession({
          sessionId: sid!,
          interactionCount: data.interactionCount || 0,
          signInPrompted: data.signInPrompted || false,
          assistantName: localName || data.assistantName || "Leo",
          currentTheme: data.currentTheme || "sky",
        });
      })
      .catch(() => {
        setSession({
          sessionId: sid!,
          interactionCount: 0,
          signInPrompted: false,
          assistantName: localStorage.getItem("daycore-assistant-name") || "Leo",
          currentTheme: "sky",
        });
      });

    // Listen for same-page name change broadcast
    const handleNameChange = (e: Event) => {
      const newName = (e as CustomEvent<string>).detail;
      if (newName) {
        setSession((prev) =>
          prev ? { ...prev, assistantName: newName } : prev
        );
      }
    };

    window.addEventListener(ASSISTANT_NAME_EVENT, handleNameChange);
    return () => window.removeEventListener(ASSISTANT_NAME_EVENT, handleNameChange);
  }, []);

  return session || {
    sessionId: null,
    interactionCount: 0,
    signInPrompted: false,
    assistantName: typeof window !== "undefined"
      ? (localStorage.getItem("daycore-assistant-name") || "Leo")
      : "Leo",
    currentTheme: "sky",
  };
}
