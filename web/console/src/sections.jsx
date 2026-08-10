// The eight sections, in the order the console shows them.
//
// Order is not the prototype's: it runs from "what is wrong right now" to "what
// can I change" to "what can I break". The overview is first because it is the
// screen somebody opens when they do not yet know what they are looking for,
// and the database browser is last because it is the one with no undo.
//
// ⚠️ Views land in this file as they are written. A section listed here without
// a view would render nothing and look like a bug, so the list and the views
// move together.
import { Config } from './sections/config.jsx';
import { Placeholder } from './sections/placeholder.jsx';
import { Providers } from './sections/providers.jsx';

export const SECTIONS = [
  { id: 'overview', label: '总览', view: Placeholder },
  { id: 'config', label: '服务配置', view: Config },
  { id: 'providers', label: '能力源', view: Providers },
  { id: 'models', label: '模型', view: Placeholder },
  { id: 'oauth', label: '第三方登录', view: Placeholder },
  { id: 'prompts', label: '提示词', view: Placeholder },
  { id: 'ai-logs', label: 'AI 日志', view: Placeholder },
  { id: 'users', label: '用户', view: Placeholder },
  { id: 'db', label: '数据库', view: Placeholder },
];
