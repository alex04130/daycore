import { useCallback, useMemo, useState } from 'react';
import * as api from '../api.js';
import { Field, Notice, Screen, Secret, useSection } from '../ui.jsx';

// 服务配置。
//
// # 这一屏最难的不是渲染，是让「改不了」看起来不像 bug
//
// 后端把每个旋钮分成启动期与运行时两层，判据是「进程有没有用它造出别的
// 东西」。启动期那些**只读**，而一个灰掉的输入框如果不解释自己，读起来就是
// 一个坏掉的界面 —— 所以每一条都带着后端给的 why，鼠标停上去就能看到。
//
// # 三件必须照做的事，全都来自端点的形状
//
//  1. `null` 重置到环境种子，与 `""` 是两个不同的请求。对字符串型旋钮，
//     「恢复默认」和「设成空」不是一回事，只有其中一个的界面撤销不了一次
//     误操作。所以「重置」是一个独立的动作，不是把输入框清空。
//  2. `requiresRestart` 是按 key 报的，不是一个全局布尔。运维接下来要问的
//     正是「我改的哪一条还在等」。
//  3. 整份请求先校验完再写（后端没有事务）。所以这一屏一次提交全部改动，
//     不逐项保存 —— 逐项保存会让一半成功一半失败，而运维无从分辨是哪一半。
export function Config({ onUnauthorized }) {
  const fetcher = useCallback(() => api.getConfig(), []);
  const state = useSection(fetcher, { onUnauthorized });

  // 待提交的改动：key → string | null（null 表示重置到种子）。
  const [draft, setDraft] = useState({});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [waiting, setWaiting] = useState([]);

  const settings = state.data?.settings || [];
  const { boot, runtime } = useMemo(() => splitByLayer(settings), [settings]);
  const dirty = Object.keys(draft).length;

  async function save() {
    setBusy(true);
    setErr('');
    try {
      const res = await api.putConfig(draft);
      setDraft({});
      // 后端按 key 告诉我们哪些还在等重启。存下来而不是当场弹一句就消失 ——
      // 那正是运维过五分钟会再问一遍的东西。
      setWaiting(res?.requiresRestart || []);
      state.reload();
    } catch (e) {
      if (e instanceof api.Unauthorized) return onUnauthorized?.();
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Screen
      title="服务配置"
      sub="每个旋钮分两层：启动期的进程已经用它造出了别的东西，改不了；运行时的下次读就生效。"
      state={state}
      actions={
        dirty > 0 && (
          <div className="save-bar">
            <button onClick={save} disabled={busy}>
              {busy ? '保存中…' : `保存 ${dirty} 项改动`}
            </button>
            <button className="linkish" onClick={() => setDraft({})} disabled={busy}>
              放弃
            </button>
          </div>
        )
      }
    >
      {err && <Notice kind="error" title="没保存成功">{err}</Notice>}
      {waiting.length > 0 && (
        <Notice kind="warn" title="这几项要重启才生效">
          <p>
            它们按性质是运行时的，但当前的值在进程启动时就被别的东西复制走了
            —— 存下来是对的，只是要等下次启动。
          </p>
          <ul className="tight">
            {waiting.map((k) => (
              <li key={k}>
                <code>{k}</code>
              </li>
            ))}
          </ul>
        </Notice>
      )}

      <h2 className="group-title">运行时（可改，下次读就生效）</h2>
      {runtime.map((s) => (
        <RuntimeRow
          key={s.key}
          setting={s}
          draft={draft}
          onChange={(v) => setDraft((d) => ({ ...d, [s.key]: v }))}
          onRevert={() => setDraft((d) => { const { [s.key]: _, ...rest } = d; return rest; })}
        />
      ))}

      <h2 className="group-title">
        启动期（只读）
        <span className="group-note">
          进程读了一次，并用它造出了别的东西 —— 一个 socket、一个数据库句柄、一把签名密钥。
          给新值改不了已经造出来的那个。
        </span>
      </h2>
      {boot.map((s) => (
        <BootRow key={s.key} setting={s} />
      ))}
    </Screen>
  );
}

function splitByLayer(settings) {
  const boot = [];
  const runtime = [];
  for (const s of settings) (s.layer === 'runtime' ? runtime : boot).push(s);
  return { boot, runtime };
}

function RuntimeRow({ setting, draft, onChange, onRevert }) {
  const staged = Object.prototype.hasOwnProperty.call(draft, setting.key);
  const value = staged ? draft[setting.key] : setting.value ?? '';
  const isReset = staged && draft[setting.key] === null;

  return (
    <div className={`card ${staged ? 'staged' : ''}`}>
      <div className="card-head">
        <div className="card-title">
          {setting.env}
          <code>{setting.key}</code>
        </div>
        <div className="badges">
          {setting.source === 'override' && (
            <span className="pill" title="这个值来自控制台的覆盖，不是环境变量">已覆盖</span>
          )}
          {setting.requiresRestart && (
            <span className="pill warn" title="按性质是运行时的，但当前的值在启动时被复制走了">要重启</span>
          )}
        </div>
      </div>

      {setting.secret ? (
        // 密钥永远不返回值，所以也没有输入框可给。改它要改环境变量。
        <Field label="值" why="密钥一律启动期：能显示它的控制台已经把它弄丢了，能改它的控制台会用一次手滑换掉所有人的会话。">
          <Secret set={setting.set} env={setting.env} />
        </Field>
      ) : (
        <Field label="值">
          {isReset ? (
            <span className="reset-note">
              将重置为环境变量的值
              <button className="linkish" onClick={onRevert}>取消</button>
            </span>
          ) : (
            <input
              value={value}
              onChange={(e) => onChange(e.target.value)}
              spellCheck={false}
            />
          )}
        </Field>
      )}

      {setting.why && <Field label="说明">{setting.why}</Field>}

      {!setting.secret && !isReset && setting.source === 'override' && (
        // 「重置」与「清空」是两个动作，因为后端把 null 和 "" 当两个请求。
        // 只提供清空的界面，撤销不了一次误操作。
        <button className="linkish" onClick={() => onChange(null)}>
          恢复为环境变量的值
        </button>
      )}
    </div>
  );
}

function BootRow({ setting }) {
  return (
    <div className="card readonly">
      <div className="card-head">
        <div className="card-title">
          {setting.env}
          <code>{setting.key}</code>
        </div>
      </div>
      <Field label="值" why={setting.why || '启动期：进程已经用它造出了别的东西。'}>
        {setting.secret ? (
          <Secret set={setting.set} env={setting.env} />
        ) : (
          <code>{setting.value || <span className="dim">（未设置）</span>}</code>
        )}
      </Field>
      {setting.why && <Field label="为什么改不了">{setting.why}</Field>}
    </div>
  );
}
