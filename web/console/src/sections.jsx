// The eight sections, in the prototype's order (design-ui/liuli/admin/admin-shell.jsx NAV).
import { AILogs } from './sections/ailogs.jsx';
import { Config } from './sections/config.jsx';
import { DB } from './sections/db.jsx';
import { Models } from './sections/models.jsx';
import { OAuth } from './sections/oauth.jsx';
import { Overview } from './sections/overview.jsx';
import { Prompts } from './sections/prompts.jsx';
import { Users } from './sections/users.jsx';

export const SECTIONS = [
  { id: 'overview', view: Overview },
  { id: 'prompts', view: Prompts },
  { id: 'models', view: Models },
  { id: 'oauth', view: OAuth },
  { id: 'config', view: Config },
  { id: 'ailogs', view: AILogs },
  { id: 'users', view: Users },
  { id: 'db', view: DB },
];
