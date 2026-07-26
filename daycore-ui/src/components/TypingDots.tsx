import { motion } from "framer-motion";
import { cn } from "../lib/cn";

export interface TypingDotsProps {
  /** Dot diameter in px. Default `6`. */
  size?: number;
  /** Dot color (CSS value). Defaults to the theme's muted text token. */
  color?: string;
  className?: string;
}

/**
 * Three bouncing dots — the "assistant is typing" / thinking indicator used in
 * the companion chat and while AI responses stream in.
 */
export function TypingDots({ size = 6, color = "var(--color-text-muted)", className }: TypingDotsProps) {
  return (
    <div className={cn("flex items-center gap-1", className)}>
      {[0, 1, 2].map((i) => (
        <motion.span
          key={i}
          className="inline-block rounded-full"
          style={{ width: size, height: size, background: color }}
          animate={{ y: [0, -size * 0.7, 0] }}
          transition={{ repeat: Infinity, duration: 0.6, delay: i * 0.18 }}
        />
      ))}
    </div>
  );
}

TypingDots.displayName = "TypingDots";
