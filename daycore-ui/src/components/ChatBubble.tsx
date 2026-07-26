import { type ReactNode } from "react";
import { cn } from "../lib/cn";
import { TypingDots } from "./TypingDots";

export type ChatRole = "user" | "assistant";

export interface ChatBubbleProps {
  /** Who is speaking. `"user"` aligns right with the brand fill; `"assistant"` left on glass. */
  role: ChatRole;
  /** Message content. Omit while `typing` to show the dots. */
  children?: ReactNode;
  /** Show the typing indicator instead of content. */
  typing?: boolean;
  className?: string;
}

/**
 * A single chat message in the companion conversation. User messages are
 * right-aligned in the brand color; assistant messages sit left on a glass
 * surface. Set `typing` for the streaming/thinking state.
 */
export function ChatBubble({ role, children, typing, className }: ChatBubbleProps) {
  const isUser = role === "user";
  return (
    <div className={cn("flex", isUser ? "justify-end" : "justify-start", className)}>
      <div
        className="max-w-[80%] px-4 py-2.5 text-sm leading-relaxed"
        style={
          isUser
            ? {
                background: "var(--color-primary)",
                color: "#fff",
                borderRadius: "16px",
                borderBottomRightRadius: 6,
              }
            : {
                background: "var(--color-surface)",
                color: "var(--color-text-primary)",
                border: "1px solid var(--color-border-custom)",
                borderRadius: "16px",
                borderBottomLeftRadius: 6,
              }
        }
      >
        {typing ? <TypingDots /> : children}
      </div>
    </div>
  );
}

ChatBubble.displayName = "ChatBubble";
