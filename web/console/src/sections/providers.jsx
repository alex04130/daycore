import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Field, Notice, Screen, Secret, useSection } from '../ui.jsx';

// 外部能力源（天气 / 搜索 / 通道）。
//
// # 这一屏要让三件事显而易见
//
//  1. **`base_url` 之类为什么改不了。** 不是还没做 —— 它是 SSRF 入口。能从
//     网页改的 base_url，等于一个被攻破的控制台可以把后端指向内网 metadata
//     地址，而答案会顺着 days[].text → 模型 → 用户的聊天窗口走出去。
//  2. **改了描述，批准会自动撤销。** 后端存的是描述的哈希，批准的意思是
//     「这段文字我读过」。不把这件事摆在编辑框旁边，运维会以为是 bug 并去
//     反复点批准。
//  3. **健康是每进程的。** 两个控制台合理地给出不同答案，所以实例名必须
//     显示 —— 否则那是一份没人能解释的 bug 报告。
export function Providers({ onUnauthorized }) {
  const fetcher = useCallback(() => api.getProviders(), []);
  const state = useSection(fetcher, { onUnauthorized });
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState('');

  const data = state.data || {};
  const list = data.providers || [];

  const write = useCallback(
    async (patch) => {
      setBusy(patch.kind + '/' + patch.id);
      setErr('');
      try {
        await api.putProviders([patch]);
        state.reload();
      } catch (e) {
        if (e instanceof api.Unauthorized) return onUnauthorized?.();
        setErr(e.message);
      } finally {
        setBusy('');
      }
    },
    [state, onUnauthorized],
  );

  return (
    <Screen
      title="外部能力源"
      sub="天气与搜索的源。可改的只有三项 —— 其余住在配置文件里，因为进程在启动时已经用它们造出了 HTTP 客户端。"
      state={state}
    >
      {err && <Notice kind="error" title="没保存成功">{err}</Notice>}

      <Notice kind="info" title="这一屏改不了什么，以及为什么">
        <p>
          <code>id</code> / <code>format</code> / <code>base_url</code> / <code>token_env</code> 只能改{' '}
          <code>{data.configPath || 'config/providers.yaml'}</code> 再重启。
        </p>
        <p>
          <code>base_url</code> 尤其是有意的：它是 SSRF 入口。能从网页改的地址，
          等于一个被攻破的控制台可以把后端指向内网，而答案会顺着天气正文 → 模型 →
          用户的聊天窗口走出去。要求登机器才能改，就是全部防线。
        </p>
      </Notice>

      {list.length === 0 && (
        <div className="placeholder empty">
          没有配置任何源。删掉 <code>providers.yaml</code> 时会按环境变量取默认源，
          所以这里为空通常意味着文件里显式写了空列表。
        </div>
      )}

      {list.map((p) => (
        <ProviderCard
          key={p.kind + '/' + p.id}
          p={p}
          busy={busy === p.kind + '/' + p.id}
          onWrite={write}
        />
      ))}

      {data.instance && (
        <p className="hint small">
          健康状态由 <code>{data.instance}</code> 这一个进程测得。多实例部署下，
          另一台可能给出不同答案 —— 适配层常与后端同机，A 够不着的源 B 可能够得着。
        </p>
      )}
    </Screen>
  );
}

function ProviderCard({ p, busy, onWrite }) {
  const [desc, setDesc] = useState(p.description || {});
  const [editing, setEditing] = useState(false);
  const changed =
    (desc['zh-CN'] || '') !== ((p.description || {})['zh-CN'] || '') ||
    (desc['en-US'] || '') !== ((p.description || {})['en-US'] || '');

  const base = { kind: p.kind, id: p.id };
  const health = p.health || {};

  return (
    <div className="card">
      <div className="card-head">
        <div className="card-title">
          {p.displayName || p.id}
          <code>
            {p.kind}/{p.id}
          </code>
        </div>
        <div className="badges">
          <span className={`pill ${health.up ? 'ok' : 'err'}`} title={health.reason || ''}>
            {health.up ? '可用' : '不可用'}
          </span>
          {!p.enabled && <span className="pill mute">已停用</span>}
          <span className="pill mute">{p.format}</span>
        </div>
      </div>

      {!health.up && health.reason && (
        <Field label="为什么不可用">
          <code className="reason">{health.reason}</code>
        </Field>
      )}

      {p.format === 'http' ? (
        <Field label="地址" why="只能改文件：进程在启动时用它造出了 HTTP 客户端；而且它是 SSRF 入口。">
          <code>{p.baseUrl}</code>
        </Field>
      ) : (
        <Field label="实现" why="编译进这个二进制的内置实现。">
          <code>{p.impl}</code>
        </Field>
      )}

      {p.tokenEnv && (
        <Field label="令牌" why="这里显示的是变量名，不是凭据。值永远不返回 —— 打码的密钥仍然告诉别人它有多长。">
          <Secret set={p.tokenSet} env={p.tokenEnv} />
        </Field>
      )}

      <Field label="启用">
        <label className="switch">
          <input
            type="checkbox"
            checked={p.enabled}
            disabled={busy}
            onChange={(e) => onWrite({ ...base, enabled: e.target.checked })}
          />
          <span>{p.enabled ? '在用' : '停用中'}</span>
        </label>
      </Field>

      <Field
        label="给模型的描述"
        why="模型按它决定什么问题用哪个源。未批准时后端会用一句由 id 和格式机械生成的话代替。"
      >
        {editing ? (
          <div className="desc-edit">
            <label>
              中文
              <input
                value={desc['zh-CN'] || ''}
                onChange={(e) => setDesc({ ...desc, 'zh-CN': e.target.value })}
                placeholder="覆盖东亚，逐小时"
              />
            </label>
            <label>
              English
              <input
                value={desc['en-US'] || ''}
                onChange={(e) => setDesc({ ...desc, 'en-US': e.target.value })}
                placeholder="East Asia, hourly"
              />
            </label>
            <p className="hint small">
              两种语言都要 —— 只有一种的描述会对另一半用户凭空消失。
            </p>
            {changed && p.approved && (
              // 这句必须出现在保存之前，不是之后。
              <Notice kind="warn" title="保存后批准会撤销">
                批准的意思是「这段文字我读过」，所以它是对着**那几个字**给的。
                改了字就要重新读一遍再批准 —— 这不是 bug。
              </Notice>
            )}
            <div className="confirm-actions">
              <button
                disabled={busy}
                onClick={() => {
                  onWrite({ ...base, description: desc });
                  setEditing(false);
                }}
              >
                保存
              </button>
              <button
                className="linkish"
                onClick={() => {
                  setDesc(p.description || {});
                  setEditing(false);
                }}
              >
                取消
              </button>
            </div>
          </div>
        ) : (
          <div className="desc-view">
            {p.description?.['zh-CN'] ? (
              <>
                <div>{p.description['zh-CN']}</div>
                <div className="dim">{p.description['en-US']}</div>
              </>
            ) : (
              <span className="dim">（没写描述，模型会拿到一句机械生成的）</span>
            )}
            <button className="linkish" onClick={() => setEditing(true)}>
              编辑
            </button>
          </div>
        )}
      </Field>

      <Field
        label="批准进提示词"
        why="没批准的描述不会进模型的提示词。这是给第三方文本的一道门。"
      >
        <label className="switch">
          <input
            type="checkbox"
            checked={p.approved}
            disabled={busy || !p.description?.['zh-CN']}
            onChange={(e) => onWrite({ ...base, approved: e.target.checked })}
          />
          <span>
            {p.approved ? '已批准，模型会读到上面那段' : '未批准，模型拿到机械生成的一句'}
          </span>
        </label>
      </Field>

      {p.manifestDescription?.['zh-CN'] && (
        <Field
          label="适配层自己的说法"
          why="这是适配层通过网络自报的。它永远不会进提示词 —— 要用就自己读一遍、抄进上面那栏、再批准。"
        >
          <div className="suggestion">
            <div>{p.manifestDescription['zh-CN']}</div>
            <button
              className="linkish"
              onClick={() => {
                setDesc(p.manifestDescription);
                setEditing(true);
              }}
            >
              抄到上面去编辑
            </button>
          </div>
        </Field>
      )}
    </div>
  );
}
