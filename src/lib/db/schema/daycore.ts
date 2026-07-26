import { pgTable, text, timestamp, integer, boolean, jsonb, uuid } from "drizzle-orm/pg-core";

// ─── Sessions (anonymous + authenticated) ───────────────────────────────────
export const sessions = pgTable("sessions", {
  id: text("id").primaryKey(), // client-generated UUID stored in localStorage
  userId: text("user_id"), // null until optional sign-in
  interactionCount: integer("interaction_count").notNull().default(0),
  signInPrompted: boolean("sign_in_prompted").notNull().default(false),
  assistantName: text("assistant_name").notNull().default("Leo"),
  currentTheme: text("current_theme").notNull().default("sky"),
  createdAt: timestamp("created_at").notNull().defaultNow(),
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
});

// ─── Day Plans ───────────────────────────────────────────────────────────────
export const dayPlans = pgTable("day_plans", {
  id: uuid("id").primaryKey().defaultRandom(),
  sessionId: text("session_id").notNull().references(() => sessions.id, { onDelete: "cascade" }),
  date: text("date").notNull(), // YYYY-MM-DD
  blocks: jsonb("blocks").notNull().default([]), // TimeBlock[]
  sourceType: text("source_type").notNull().default("text"), // "text" | "image"
  note: text("note"), // AI's warm note
  createdAt: timestamp("created_at").notNull().defaultNow(),
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
});

// ─── Mood Check-ins ──────────────────────────────────────────────────────────
export const moodCheckins = pgTable("mood_checkins", {
  id: uuid("id").primaryKey().defaultRandom(),
  sessionId: text("session_id").notNull().references(() => sessions.id, { onDelete: "cascade" }),
  mood: text("mood").notNull(), // "stressed" | "anxious" | "tired" | "okay" | "great"
  aiResponse: text("ai_response"),
  exerciseOffered: text("exercise_offered"), // "breathing" | "stretch" | "grounding"
  exerciseCompleted: boolean("exercise_completed").notNull().default(false),
  theme: text("theme"), // theme active at time of check-in
  createdAt: timestamp("created_at").notNull().defaultNow(),
});

// ─── Companion Memory ────────────────────────────────────────────────────────
export const companionMemory = pgTable("companion_memory", {
  id: uuid("id").primaryKey().defaultRandom(),
  sessionId: text("session_id").notNull().references(() => sessions.id, { onDelete: "cascade" }),
  keyFacts: jsonb("key_facts").notNull().default([]), // string[]
  conversationHistory: jsonb("conversation_history").notNull().default([]), // {role, content}[]
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
});

// ─── Theme Switch Log (for AI mood signal) ───────────────────────────────────
export const themeSwitchLog = pgTable("theme_switch_log", {
  id: uuid("id").primaryKey().defaultRandom(),
  sessionId: text("session_id").notNull().references(() => sessions.id, { onDelete: "cascade" }),
  theme: text("theme").notNull(),
  switchedAt: timestamp("switched_at").notNull().defaultNow(),
});

// ─── Types ───────────────────────────────────────────────────────────────────
export type TimeBlock = {
  id: string;
  date?: string;           // YYYY-MM-DD，AI 返回时携带，表示这个块属于哪天
  time: string | null;     // HH:MM，无明确时间时为 null
  title: string;
  type: "task" | "appointment" | "break" | "relax" | "meal";
  duration_min?: number;
  time_mode: "floating" | "fixed";
  timezone: string;
  completed?: boolean;
  isAchievement?: boolean;
};

export type Message = {
  role: "user" | "assistant";
  content: string;
  timestamp: string;
};
