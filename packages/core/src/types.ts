// The wire types. Every one of them is a shape api/openapi.yaml defines — this
// file is a hand-maintained mirror, so a field added there and not here is a
// field the frontends silently cannot see.

export interface TimeBlock {
  id: string;
  date?: string;
  time: string | null;
  title: string;
  type: 'task' | 'appointment' | 'break' | 'relax' | 'meal';
  duration_min: number | null;
  completed?: boolean;
  isAchievement?: boolean;
  origin?: 'auto' | 'manual' | 'rule';
  hidden?: boolean;
  lockLevel?: string;
  lockReason?: string;
  note?: string;
}

export interface DayPlan {
  date: string;
  blocks: TimeBlock[];
  note?: string;
}

export interface Proposal {
  id: string;
  state: 'pending' | 'accepted' | 'rejected' | 'expired';
  level: 'L1' | 'L2' | 'L3';
  kind: 'timed' | 'card' | 'decision';
  title: string;
  summary?: string;
  reason?: string;
  evidence?: string;
  date?: string;
  start?: string;
  dur?: number | null;
  btype?: string;
  expiresAt?: string;
}

export interface Session {
  id: string;
  assistantName: string;
  currentTheme: string;
  language?: string;
  sessionToken?: string;
}

export interface OperationLog {
  id: string;
  action: string;
  summary?: string;
  reverted?: boolean;
  revertedBy?: string;
  createdAt: string;
}

/** What POST /api/version answers. Fields the backend may omit are optional. */
export interface Handshake {
  apiVersion?: number;
  apiMinor?: number;
  version?: string;
  assignedFamilyId: string;
  handshakeRecorded: boolean;
  rulesAccepted?: boolean;
  newTokens?: string[];
  /** Kinds 汀 proposed that are waiting on a human — see manifest.ts. */
  pendingKinds?: string[];
  /** Tokens held back because their kind is still pending. */
  deferredTokens?: string[];
  pendingThemeBackfill?: number;
  note?: string;
  /** Every language this INSTALLATION can render — not a compile-time list.
   *  An operator dropping a file into LOCALES_DIR extends it. */
  locales?: { available: string[]; defaultPrimary: string; defaultSecondary?: string };
}
