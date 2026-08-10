import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Confirm, Empty, Notice, Screen, useSection } from '../ui.jsx';

// Users and groups.
//
// # One screen, because they are one question
//
// "Who can export the database" is answered by two facts that live apart: what
// a group grants, and who is in it. Splitting them across two screens means the
// question can only be answered by holding one screen in your head while
// reading the other — which is how somebody ends up granting a group without
// noticing it already has eleven members.
//
// So the group editor shows its member count next to its permissions, and the
// user list shows each person's groups. Neither screen ever shows a group name
// without saying what it means.
//
// # What is deliberately NOT here
//
// A "permissions" column per user. It would be the union of their groups, and
// showing it invites editing it — which is exactly the per-user grant the model
// does not have. The union is a derived thing; the groups are the fact.
export function Users({ onUnauthorized, principal }) {
  // Three reads in one, because the screen is meaningless with any of them
  // missing: a group name with no permission list, or a permission id with no
  // damage line, is a decision nobody can make.
  const load = useCallback(
    () =>
      Promise.all([api.getUsers(), api.getRoles(), api.getPermissions()]).then(
        ([users, roles, perms]) => ({
          ...users,
          roles: roles.roles,
          permissions: perms.permissions,
        }),
      ),
    [],
  );
  const { reload, ...state } = useSection(load, { onUnauthorized });

  return (
    <Screen
      title="用户与权限组"
      sub="组决定能做什么，成员决定谁能做。两者分开授予，这不是麻烦，是防止「能改组」和「能进组」落到同一个人手上。"
      state={state}
    >
      {state.data && (
        <>
          <RoleTable
            roles={state.data.roles}
            permissions={state.data.permissions}
            principal={principal}
            onChanged={reload}
          />
          <UserTable
            users={state.data.users}
            ownerCount={state.data.ownerCount}
            roles={state.data.roles}
            onChanged={reload}
          />
        </>
      )}
    </Screen>
  );
}

function RoleTable({ roles, permissions, principal, onChanged }) {
  const [editing, setEditing] = useState(null);
  const [creating, setCreating] = useState(false);

  return (
    <div className="block">
      <div className="block-head">
        <h2>权限组</h2>
        <button onClick={() => setCreating(true)}>新建组</button>
      </div>
      <p className="sub">
        没有任何权限的组就是「普通用户组」—— 套餐档位是这么表达的。带权限的组是管理员组，
        改它的成员需要「改组权限」这一项。
      </p>
      {creating && (
        <RoleEditor
          role={{ name: '', description: '', permissions: [] }}
          permissions={permissions}
          isNew
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false);
            onChanged();
          }}
        />
      )}
      {roles.length === 0 && !creating && <Empty>还没有建过组。</Empty>}
      {roles.map((r) =>
        editing === r.name ? (
          <RoleEditor
            key={r.name}
            role={r}
            permissions={permissions}
            onClose={() => setEditing(null)}
            onSaved={() => {
              setEditing(null);
              onChanged();
            }}
          />
        ) : (
          <div key={r.name} className="row role-row">
            <div className="row-main">
              <div className="row-title">
                <code>{r.name}</code>
                <span className={`pill ${r.admin ? 'warn' : ''}`}>
                  {r.admin ? '管理员组' : '普通用户组'}
                </span>
                <span className="muted">{r.members.length} 人</span>
              </div>
              {r.description && <div className="sub">{r.description}</div>}
              <div className="perm-chips">
                {r.permissions.length === 0 ? (
                  <span className="muted">不授予任何控制台权限</span>
                ) : (
                  r.permissions.map((p) => (
                    <code key={p} className="chip" title={damageOf(permissions, p)}>
                      {p}
                    </code>
                  ))
                )}
              </div>
            </div>
            <div className="row-actions">
              <button onClick={() => setEditing(r.name)}>编辑</button>
              <Confirm
                word={r.name}
                label="删除"
                danger={`删掉这个组，同时会把 ${r.members.length} 个人从里面移出去。`}
                onConfirm={() => api.deleteRole(r.name).then(onChanged, (e) => alert(e.message))}
              />
            </div>
          </div>
        ),
      )}
      {principal && !principal.permissions.includes('roles.edit') && !principal.root && (
        <Notice kind="info" title="只读">
          你没有「改组权限」这一项，所以这里的编辑与删除会被服务端拒绝。这是有意的：
          能改组权限的人可以把自己加进一个全权限的组。
        </Notice>
      )}
    </div>
  );
}

function damageOf(permissions, id) {
  return permissions.find((p) => p.id === id)?.damage || id;
}

// The role editor: the permission list with its damage line beside every
// switch.
//
// The damage line is not a tooltip here. The person granting is deciding what
// somebody else can break, usually quickly, and a sentence they have to hover
// to see is a sentence they will not read.
function RoleEditor({ role, permissions, isNew, onClose, onSaved }) {
  const [name, setName] = useState(role.name);
  const [description, setDescription] = useState(role.description || '');
  const [set, setSet] = useState(() => new Set(role.permissions));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const toggle = (id) =>
    setSet((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  async function save() {
    setBusy(true);
    setErr('');
    try {
      await api.putRole(name.trim(), { description, permissions: [...set] });
      onSaved();
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  }

  const becomingAdmin = set.size > 0 && role.permissions.length === 0;

  return (
    <div className="row role-editor">
      <div className="field">
        <div className="field-label">组名</div>
        <div className="field-value">
          {isNew ? (
            <input value={name} onChange={(e) => setName(e.target.value)} autoFocus maxLength={64} />
          ) : (
            <code>{name}</code>
          )}
        </div>
      </div>
      <div className="field">
        <div className="field-label">说明</div>
        <div className="field-value">
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="半年后还看得懂这个组是干什么的"
          />
        </div>
      </div>

      {set.has('roles.edit') && (
        <Notice kind="warn" title="这一项等同于超级管理员">
          有「改组权限」的人可以把自己加进一个全权限的组。给出它和把人设成 owner 是同一个决定。
        </Notice>
      )}
      {becomingAdmin && (
        <Notice kind="warn" title="这个组要从普通用户组变成管理员组">
          保存后，把人放进这个组就不再只需要「分配用户」，还需要「改组权限」。
          已经在组里的 {role.members?.length ?? 0} 个人会立刻拿到这些权限。
        </Notice>
      )}

      <div className="perm-grid">
        {permissions.map((p) => (
          <label key={p.id} className={`perm ${set.has(p.id) ? 'on' : ''}`}>
            <input type="checkbox" checked={set.has(p.id)} onChange={() => toggle(p.id)} />
            <div>
              <code>{p.id}</code>
              <div className="sub">{p.damage}</div>
            </div>
          </label>
        ))}
      </div>

      {err && <Notice kind="error">{err}</Notice>}
      <div className="row-actions">
        <button disabled={busy || !name.trim()} onClick={save}>
          {busy ? '…' : '保存'}
        </button>
        <button className="linkish" onClick={onClose}>
          取消
        </button>
      </div>
    </div>
  );
}

function UserTable({ users, ownerCount, roles, onChanged }) {
  return (
    <div className="block">
      <div className="block-head">
        <h2>用户</h2>
        <span className="muted">{ownerCount} 个 owner</span>
      </div>
      <p className="sub">
        owner 是「破窗」标记，不是一个装满权限的组 —— 以后加的每一项权限，owner 当天就有。
        它只能用 <code>ADMIN_TOKEN</code> 设置：第一个管理员得从外面指定，而这也是所有人都被锁在外面时唯一的进路。
      </p>
      {users.length === 0 && <Empty>还没有用户。</Empty>}
      {users.map((u) => (
        <UserRow key={u.id} user={u} roles={roles} ownerCount={ownerCount} onChanged={onChanged} />
      ))}
    </div>
  );
}

function UserRow({ user, roles, ownerCount, onChanged }) {
  const [set, setSet] = useState(() => new Set(user.roles));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const dirty =
    set.size !== user.roles.length || user.roles.some((r) => !set.has(r));

  async function saveRoles() {
    setBusy(true);
    setErr('');
    try {
      await api.putUserRoles(user.id, [...set]);
      onChanged();
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  }

  async function toggleOwner() {
    setErr('');
    try {
      await api.putUserOwner(user.id, !user.owner);
      onChanged();
    } catch (e) {
      setErr(e.message);
    }
  }

  return (
    <div className="row user-row">
      <div className="row-main">
        <div className="row-title">
          <strong>{user.name || user.email || user.id}</strong>
          {user.owner && <span className="pill warn">owner</span>}
        </div>
        <div className="sub">
          {user.email && <span>{user.email} · </span>}
          <code className="muted">{user.id}</code>
          {user.createdAt && <span className="muted"> · {user.createdAt.slice(0, 10)}</span>}
        </div>
        <div className="perm-chips">
          {roles.length === 0 ? (
            <span className="muted">还没有可分配的组</span>
          ) : (
            roles.map((r) => (
              <label key={r.name} className={`chip toggle ${set.has(r.name) ? 'on' : ''}`}>
                <input
                  type="checkbox"
                  checked={set.has(r.name)}
                  onChange={() =>
                    setSet((prev) => {
                      const next = new Set(prev);
                      if (next.has(r.name)) next.delete(r.name);
                      else next.add(r.name);
                      return next;
                    })
                  }
                />
                {r.name}
                {r.admin && <span className="pill warn small">管</span>}
              </label>
            ))
          )}
        </div>
        {err && <Notice kind="error">{err}</Notice>}
      </div>
      <div className="row-actions">
        {dirty && (
          <button disabled={busy} onClick={saveRoles}>
            {busy ? '…' : '保存组'}
          </button>
        )}
        {user.owner ? (
          <Confirm
            word={user.id}
            label="取消 owner"
            danger={
              ownerCount <= 1
                ? '这是最后一个 owner。取消之后，只有 ADMIN_TOKEN 还能再指定一个 —— 那条路一直在，但除此之外没有别的。'
                : '这个人会失去全部权限，除非他还在别的组里。'
            }
            onConfirm={toggleOwner}
          />
        ) : (
          <button onClick={toggleOwner} title="只有 ADMIN_TOKEN 能做这件事">
            设为 owner
          </button>
        )}
      </div>
    </div>
  );
}
