"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Calendar, MessageCircle, Heart, Grid, Settings, LogOut, UserRound, X } from "lucide-react";
import { ThemeSwitcher } from "@/components/layout/theme-switcher";
import { useTheme } from "@/lib/theme-context";
import { cn } from "@/utils/utils";
import { auth } from "@eazo/sdk";
import { useEazo } from "@eazo/sdk/react";

const TABS = [
  { href: "/", icon: Calendar, label: "今日" },
  { href: "/companion", icon: MessageCircle, label: "Leo" },
  { href: "/mood", icon: Heart, label: "心情" },
  { href: "/life", icon: Grid, label: "生活" },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { theme } = useTheme();
  const isFullscreen = pathname.startsWith("/mood/exercise");

  return (
    <div className="flex min-h-svh" data-theme={theme}>
      {/* ── Desktop sidebar ── */}
      <aside
        className="hidden lg:flex flex-col w-60 shrink-0 fixed inset-y-0 left-0 z-40 border-r"
        style={{
          background: "var(--tab-bar-bg)",
          backdropFilter: "blur(24px)",
          borderColor: "var(--color-border-custom)",
        }}>
        {/* Logo */}
        <div className="px-6 py-5">
          <span className="text-xl font-bold" style={{ color: "var(--color-primary)" }}>
            Daycore
          </span>
        </div>

        {/* Theme switcher */}
        <div className="px-4 mb-4">
          <ThemeSwitcher variant="sidebar" />
        </div>

        {/* Nav links */}
        <nav className="flex-1 px-3 space-y-1">
          {TABS.map(({ href, icon: Icon, label }) => {
            const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
            return (
              <Link key={href} href={href}
                className="flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-150"
                style={{
                  color: active ? "var(--color-primary)" : "var(--color-text-secondary)",
                  background: active
                    ? "color-mix(in srgb, var(--color-primary) 10%, transparent)"
                    : "transparent",
                  fontWeight: active ? 600 : 400,
                }}>
                <Icon size={20} />
                <span className="text-sm">{label}</span>
              </Link>
            );
          })}
        </nav>

        {/* Bottom: settings + user — popup goes UPWARD */}
        <div className="px-3 pb-4 space-y-1">
          <Link href="/settings"
            className="flex items-center gap-3 px-4 py-3 rounded-xl hover:opacity-80 transition-all"
            style={{ color: "var(--color-text-muted)" }}>
            <Settings size={18} />
            <span className="text-sm">设置</span>
          </Link>
          <DesktopUserButton />
        </div>
      </aside>

      {/* ── Main content ── */}
      <main className="flex-1 lg:ml-60 flex flex-col min-h-svh">
        {/* Mobile top bar */}
        {!isFullscreen && (
          <header
            className="lg:hidden shrink-0 flex items-center justify-between px-4 pt-12 pb-3"
            style={{ background: "transparent" }}>
            <span className="text-lg font-bold" style={{ color: "var(--color-primary)" }}>
              Daycore
            </span>
            <div className="flex items-center gap-2">
              <ThemeSwitcher variant="topbar" />
              <MobileUserButton />
            </div>
          </header>
        )}

        <div className={cn("flex-1", !isFullscreen && "pb-[calc(72px+env(safe-area-inset-bottom))] lg:pb-6")}>
          {children}
        </div>
      </main>

      {/* ── Mobile bottom tab bar ── */}
      {!isFullscreen && (
        <nav
          className="tab-bar fixed bottom-0 inset-x-0 z-40 lg:hidden flex"
          style={{ paddingBottom: "env(safe-area-inset-bottom)" }}>
          {TABS.map(({ href, icon: Icon, label }) => {
            const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
            return (
              <Link key={href} href={href}
                className="flex-1 flex flex-col items-center justify-center py-2 gap-0.5 min-h-[56px]">
                <motion.div
                  animate={{ scale: active ? 1.1 : 1 }}
                  transition={{ type: "spring", stiffness: 400, damping: 25 }}>
                  <Icon
                    size={22}
                    strokeWidth={active ? 2.5 : 1.8}
                    style={{ color: active ? "var(--color-primary)" : "var(--color-text-muted)" }}
                  />
                </motion.div>
                <span
                  className="text-[10px] font-medium"
                  style={{ color: active ? "var(--color-primary)" : "var(--color-text-muted)" }}>
                  {label}
                </span>
              </Link>
            );
          })}
        </nav>
      )}
    </div>
  );
}

// ── Desktop user button (popup opens UPWARD) ────────────────────────────────
function DesktopUserButton() {
  const user = useEazo((s) => s.auth.user);
  const loading = useEazo((s) => s.auth.loading);
  const [open, setOpen] = useState(false);

  if (loading) {
    return (
      <div className="px-4 py-3 flex items-center gap-3">
        <div className="w-7 h-7 rounded-full skeleton" />
        <div className="skeleton h-3 w-16 rounded" />
      </div>
    );
  }

  if (!user) {
    return (
      <button
        onClick={() => auth.login().catch(() => {})}
        className="flex items-center gap-3 px-4 py-3 rounded-xl w-full transition-all hover:opacity-80"
        style={{ color: "var(--color-primary)" }}>
        <UserRound size={18} />
        <span className="text-sm">登录 / 注册</span>
      </button>
    );
  }

  return (
    <div className="relative">
      {/* Popup — direction UP */}
      <AnimatePresence>
        {open && (
          <>
            <motion.div
              className="fixed inset-0 z-40"
              onClick={() => setOpen(false)}
            />
            <motion.div
              initial={{ opacity: 0, y: 8, scale: 0.95 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 8, scale: 0.95 }}
              transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
              className="absolute bottom-full left-0 right-0 mb-2 z-50 rounded-2xl overflow-hidden shadow-xl"
              style={{
                background: "var(--color-surface)",
                backdropFilter: "blur(20px)",
                border: "1px solid var(--color-border-custom)",
              }}>
              {/* User info */}
              <div className="px-4 py-3 border-b" style={{ borderColor: "var(--color-border-custom)" }}>
                <div className="flex items-center gap-3">
                  <Avatar user={user} size={32} />
                  <div className="min-w-0">
                    {user.name && (
                      <p className="text-sm font-semibold truncate" style={{ color: "var(--color-text-primary)" }}>
                        {user.name}
                      </p>
                    )}
                    <p className="text-xs truncate" style={{ color: "var(--color-text-muted)" }}>
                      {user.email || "已登录"}
                    </p>
                  </div>
                </div>
              </div>
              {/* Sign out */}
              <button
                onClick={() => { auth.logout(); setOpen(false); }}
                className="flex items-center gap-3 w-full px-4 py-3 text-sm transition-opacity hover:opacity-70"
                style={{ color: "var(--color-states-error)" }}>
                <LogOut size={15} />
                退出登录
              </button>
            </motion.div>
          </>
        )}
      </AnimatePresence>

      {/* Trigger button */}
      <button
        onClick={() => setOpen(v => !v)}
        className="flex items-center gap-3 px-3 py-2.5 rounded-xl w-full transition-all hover:opacity-80"
        style={{
          background: open
            ? "color-mix(in srgb, var(--color-primary) 10%, transparent)"
            : "transparent",
        }}>
        <Avatar user={user} size={28} />
        <span className="text-sm truncate flex-1 text-left" style={{ color: "var(--color-text-primary)" }}>
          {user.name || user.email || "我的账号"}
        </span>
      </button>
    </div>
  );
}

// ── Mobile user button (bottom sheet) ───────────────────────────────────────
function MobileUserButton() {
  const user = useEazo((s) => s.auth.user);
  const loading = useEazo((s) => s.auth.loading);
  const [open, setOpen] = useState(false);

  if (loading) {
    return <div className="w-8 h-8 rounded-full skeleton" />;
  }

  if (!user) {
    return (
      <button
        onClick={() => auth.login().catch(() => {})}
        className="w-9 h-9 rounded-full glass-card flex items-center justify-center"
        style={{ color: "var(--color-primary)" }}>
        <UserRound size={18} />
      </button>
    );
  }

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="w-9 h-9 rounded-full overflow-hidden glass-card">
        <Avatar user={user} size={36} />
      </button>

      <AnimatePresence>
        {open && (
          <>
            <motion.div
              initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
              className="fixed inset-0 bg-black/30 z-50"
              onClick={() => setOpen(false)}
            />
            <motion.div
              initial={{ y: "100%" }} animate={{ y: 0 }} exit={{ y: "100%" }}
              transition={{ type: "spring", stiffness: 300, damping: 30 }}
              className="fixed inset-x-0 bottom-0 z-50 rounded-t-3xl overflow-hidden"
              style={{
                background: "var(--color-surface)",
                backdropFilter: "blur(24px)",
                borderTop: "1px solid var(--color-border-custom)",
                paddingBottom: "env(safe-area-inset-bottom)",
              }}>
              {/* Handle */}
              <div className="flex justify-center pt-3 pb-1">
                <div className="w-10 h-1.5 rounded-full opacity-40"
                  style={{ background: "var(--color-text-muted)" }} />
              </div>

              {/* User info */}
              <div className="px-5 py-4 flex items-center gap-4 border-b"
                style={{ borderColor: "var(--color-border-custom)" }}>
                <Avatar user={user} size={48} />
                <div className="min-w-0">
                  {user.name && (
                    <p className="text-base font-semibold" style={{ color: "var(--color-text-primary)" }}>
                      {user.name}
                    </p>
                  )}
                  <p className="text-sm" style={{ color: "var(--color-text-muted)" }}>
                    {user.email || "已登录"}
                  </p>
                </div>
                <button
                  onClick={() => setOpen(false)}
                  className="ml-auto w-8 h-8 rounded-full flex items-center justify-center"
                  style={{ background: "color-mix(in srgb, var(--color-text-muted) 12%, transparent)" }}>
                  <X size={15} style={{ color: "var(--color-text-muted)" }} />
                </button>
              </div>

              {/* Actions */}
              <div className="px-4 py-3">
                <Link href="/settings" onClick={() => setOpen(false)}
                  className="flex items-center gap-3 px-4 py-3 rounded-xl"
                  style={{ color: "var(--color-text-secondary)" }}>
                  <Settings size={18} />
                  <span className="text-sm">设置</span>
                </Link>
                <button
                  onClick={() => { auth.logout(); setOpen(false); }}
                  className="flex items-center gap-3 px-4 py-3 rounded-xl w-full"
                  style={{ color: "var(--color-states-error)" }}>
                  <LogOut size={18} />
                  <span className="text-sm">退出登录</span>
                </button>
              </div>
            </motion.div>
          </>
        )}
      </AnimatePresence>
    </>
  );
}

// ── Avatar helper ────────────────────────────────────────────────────────────
function Avatar({ user, size }: { user: { name?: string | null; email?: string | null; avatarUrl?: string | null }; size: number }) {
  const initial = (user.name ?? user.email ?? "?")[0].toUpperCase();

  if (user.avatarUrl) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={user.avatarUrl}
        alt={initial}
        width={size}
        height={size}
        referrerPolicy="no-referrer"
        className="rounded-full object-cover"
        style={{ width: size, height: size, flexShrink: 0 }}
        onError={(e) => {
          // Fallback to initials on load error
          (e.target as HTMLImageElement).style.display = "none";
        }}
      />
    );
  }

  return (
    <div
      className="rounded-full flex items-center justify-center text-white font-semibold shrink-0"
      style={{
        width: size,
        height: size,
        background: "var(--color-primary)",
        fontSize: size * 0.4,
      }}>
      {initial}
    </div>
  );
}
