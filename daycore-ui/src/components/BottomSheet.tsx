import { type ReactNode } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { X } from "lucide-react";
import { cn } from "../lib/cn";

export interface BottomSheetProps {
  /** Whether the sheet is shown. */
  open: boolean;
  /** Fired when the backdrop or close button is pressed. */
  onClose?: () => void;
  /** Title in the sheet header. Omit to hide the header row. */
  title?: ReactNode;
  /** Hide the round close button in the header. */
  hideClose?: boolean;
  /** Sheet body. */
  children?: ReactNode;
  className?: string;
}

/**
 * A modal sheet that slides up from the bottom on a frosted surface with a grab
 * handle — the primary mobile modal (day-input, account menu, pickers). Drives
 * its own backdrop and spring animation; control via `open` / `onClose`.
 */
export function BottomSheet({ open, onClose, title, hideClose, children, className }: BottomSheetProps) {
  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 bg-black/30"
            onClick={onClose}
          />
          <motion.div
            initial={{ y: "100%" }}
            animate={{ y: 0 }}
            exit={{ y: "100%" }}
            transition={{ type: "spring", stiffness: 300, damping: 30 }}
            className={cn("fixed inset-x-0 bottom-0 z-50 overflow-hidden", className)}
            style={{
              background: "var(--color-surface)",
              backdropFilter: "blur(24px)",
              WebkitBackdropFilter: "blur(24px)",
              borderTop: "1px solid var(--color-border-custom)",
              borderTopLeftRadius: "var(--radius-sheet)",
              borderTopRightRadius: "var(--radius-sheet)",
              paddingBottom: "env(safe-area-inset-bottom)",
            }}
          >
            <div className="flex justify-center pb-1 pt-3">
              <div className="h-1.5 w-10 rounded-full opacity-40" style={{ background: "var(--color-text-muted)" }} />
            </div>
            <div className="px-5 pb-6">
              {(title || !hideClose) && (
                <div className="mb-4 flex items-center justify-between">
                  <h3 className="text-lg font-semibold" style={{ color: "var(--color-text-primary)" }}>
                    {title}
                  </h3>
                  {!hideClose && (
                    <button
                      onClick={onClose}
                      aria-label="关闭"
                      className="flex h-8 w-8 items-center justify-center rounded-full"
                      style={{ background: "color-mix(in srgb, var(--color-text-muted) 12%, transparent)" }}
                    >
                      <X size={16} style={{ color: "var(--color-text-muted)" }} />
                    </button>
                  )}
                </div>
              )}
              {children}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}

BottomSheet.displayName = "BottomSheet";
