// The ten sections, in the order the console shows them.
//
// Order is not the prototype's: it runs from "what is wrong right now" to "what
// can I change" to "what can I break". The overview is first because it is the
// screen somebody opens when they do not yet know what they are looking for,
// and the database browser is last because it is the one with no undo.
//
// ⚠️ Views land in this file as they are written. A section listed here without
// a view would render nothing and look like a bug, so the list and the views
// move together.
//
// # `need` hides, it does not protect
//
// Each section names the permission its first read requires, and the shell
// leaves out the ones the caller does not hold. That is a COURTESY — every
// endpoint checks for itself, and a console that lied to itself about what it
// holds would get 403s rather than access. What it buys is that somebody with
// users.read does not click "数据库" and get an error page as their welcome.
//
// A section with `need: null` is shown to anybody holding a console session.
import { AILogs } from './sections/ailogs.jsx';
import { Config } from './sections/config.jsx';
import { DB } from './sections/db.jsx';
import { Frontends } from './sections/frontends.jsx';
import { Overview } from './sections/overview.jsx';
import { Pairings } from './sections/pairings.jsx';
import { Placeholder } from './sections/placeholder.jsx';
import { Prompts } from './sections/prompts.jsx';
import { Providers } from './sections/providers.jsx';
import { Users } from './sections/users.jsx';

export const SECTIONS = [
  { id: 'overview', label: '总览', view: Overview, need: 'overview.read' },
  { id: 'config', label: '服务配置', view: Config, need: 'config.read' },
  { id: 'providers', label: '能力源', view: Providers, need: 'providers.read' },
  { id: 'models', label: '模型', view: Placeholder, need: 'models.read' },
  { id: 'oauth', label: '第三方登录', view: Placeholder, need: 'oauth.read' },
  { id: 'prompts', label: '提示词', view: Prompts, need: 'prompts.read' },
  { id: 'ai-logs', label: 'AI 日志', view: AILogs, need: 'ailogs.read' },
  { id: 'frontends', label: '前端', view: Frontends, need: 'frontends.read' },
  { id: 'users', label: '用户与权限', view: Users, need: 'users.read' },
  { id: 'pairings', label: '集群与外部控制台', view: Pairings, need: 'pairings.read' },
  { id: 'db', label: '数据库', view: DB, need: 'db.operational' },
];

// visibleSections is what this principal may see.
//
// An empty result is a real state — somebody whose only permission belongs to a
// section that has not been written yet — and the shell says so rather than
// rendering an empty frame.
export function visibleSections(principal) {
  if (!principal) return [];
  return SECTIONS.filter((s) => !s.need || principal.permissions.includes(s.need));
}
