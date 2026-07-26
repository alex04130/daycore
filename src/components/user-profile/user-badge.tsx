"use client";

import { useRef, useState, useEffect } from "react";
import { LogOut, UserRound, ChevronUp, ChevronDown } from "lucide-react";
import { auth } from "@eazo/sdk";
import { useEazo } from "@eazo/sdk/react";

interface Props {
  /** 下拉方向：sidebar 底部向上，topbar 向下 */
  direction?: "up" | "down";
}

export function UserBadge({ direction = "down" }: Props) {
  const user = useEazo((s) => s.auth.user);
  const loading = useEazo((s) => s.auth.loading);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", handle);
    return () => document.removeEventListener("mousedown", handle);
  }, []);

  if (loading) {
    return (
      <div className="flex h-9 w-full items-center gap-2 rounded-xl px-3"
        style={{ background: "color-mix(in srgb, var(--color-primary) 8%, transparent)" }}>
        <div className="size-4 rounded-full animate-pulse"
          style={{ background: "var(--color-border-custom)" }} />
        <div className="h-3 w-20 rounded-full animate-pulse"
          style={{ background: "var(--color-border-custom)" }} />
      </div>
    );
  }

  if (!user) {
    return (
      <button
        onClick={() => auth.login().catch(() => undefined)}
        className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition-all hover:opacity-80"
        style={{
          background: "color-mix(in srgb, var(--color-primary) 10%, transparent)",
          color: "var(--color-primary)",
        }}>
        <UserRound size={16} />
        <span>登录 / 注册</span>
      </button>
    );
  }

  const displayName = user.name || user.email?.split("@")[0] || "用户";
  const initials = displayName[0].toUpperCase();

  return (
    <div ref={ref} className="relative w-full">
      {/* Trigger */}
      <button
        onClick={() => setOpen(v => !v)}
        className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-sm transition-all hover:opacity-80"
        style={{
          background: open
            ? "color-mix(in srgb, var(--color-primary) 12%, transparent)"
            : "transparent",
          color: "var(--color-text-secondary)",
        }}>

        {/* Avatar */}
        <Avatar user={user} size={26} />

        <span className="flex-1 text-left truncate text-sm"
          style={{ color: "var(--color-text-primary)" }}>
          {displayName}
        </span>
        {direction === "up"
          ? <ChevronUp size={14} style={{ color: "var(--color-text-muted)" }} />
          : <ChevronDown size={14} style={{ color: "var(--color-text-muted)" }} />
        }
      </button>

      {/* Dropdown panel */}
      {open && (
        <div
          className="absolute left-0 right-0 z-50 rounded-2xl overflow-hidden shadow-2xl"
          style={{
            // 向上展开 or 向下展开
            ...(direction === "up"
              ? { bottom: "calc(100% + 6px)" }
              : { top: "calc(100% + 6px)" }),
            // 实心背景，不用 bg-background（Daycore 主题里不可靠）
            background: "var(--color-bg-end)",
            backdropFilter: "blur(20px)",
            WebkitBackdropFilter: "blur(20px)",
            border: "1.5px solid var(--color-border-custom)",
            boxShadow: "0 16px 48px rgba(0,0,0,0.18)",
            minWidth: 220,
          }}>

          {/* User info header */}
          <div className="px-4 py-3 flex items-center gap-3"
            style={{ borderBottom: "1px solid var(--color-border-custom)" }}>
            <Avatar user={user} size={36} />
            <div className="min-w-0">
              <p className="text-sm font-semibold truncate"
                style={{ color: "var(--color-text-primary)" }}>
                {displayName}
              </p>
              {user.email && (
                <p className="text-xs truncate"
                  style={{ color: "var(--color-text-muted)" }}>
                  {user.email}
                </p>
              )}
            </div>
          </div>

          {/* Actions */}
          <div className="px-2 py-2">
            <button
              onClick={() => { auth.logout(); setOpen(false); }}
              className="flex w-full items-center gap-2 px-3 py-2 rounded-xl text-sm transition-all hover:opacity-80"
              style={{
                color: "var(--color-states-error)",
                background: "color-mix(in srgb, var(--color-states-error) 8%, transparent)",
              }}>
              <LogOut size={14} />
              退出登录
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Avatar sub-component ─────────────────────────────────────────────────────
function Avatar({ user, size }: { user: { name?: string | null; email?: string | null; avatarUrl?: string | null }; size: number }) {
  const [imgError, setImgError] = useState(false);
  const displayName = user.name || user.email?.split("@")[0] || "?";
  const initials = displayName[0].toUpperCase();

  if (user.avatarUrl && !imgError) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={user.avatarUrl}
        alt={displayName}
        referrerPolicy="no-referrer"
        onError={() => setImgError(true)}
        className="rounded-full object-cover shrink-0"
        style={{ width: size, height: size }}
      />
    );
  }

  return (
    <div
      className="rounded-full flex items-center justify-center shrink-0 font-semibold text-white"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.4,
        background: "var(--color-primary)",
      }}>
      {initials}
    </div>
  );
}
