import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Field, Notice, Screen, Secret, useSection } from '../ui.jsx';

// 外部能力源（天气 / 搜索 / 通道）。
//
// # 这一屏要让三件事显而易见
//
//  1. **改地址会断，所以改之前要说。** 地址是可改的（2026-08-09 起），但一个
//     打错的地址不会当场报错 —— 它会在下一次有人问天气时才失败。所以编辑框
//     旁边直接说这件事，而不是等健康状态自己变红。
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

      <Notice kind="info" title="这一屏改不了什么">
        <p>
          <code>id</code> / <code>format</code> / <code>impl</code> / <code>token_env</code> 只能改{' '}
          <code>{data.configPath || 'config/providers.yaml'}</code> 再重启 ——
          进程在启动时用它们造出了别的东西。
        </p>
        <p>
          地址可以在这里改。改完立刻生效，而且**这个源的健康记录会清空** ——
          之前那些成败属于旧地址那台机器，不属于新的这台。
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
        <BaseURLField p={p} busy={busy} onWrite={onWrite} base={base} />
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
              <p className="hint small">保存后需要重新批准 —— 批准是对着那几个字给的。</p>
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
        {/*
          描述有未保存改动时这个开关是禁用的，而不是旁边挂一句警告。
          警告只是一句话，而这个动作的含义是「这段文字我读过」——
          对着一段还没定下来的文字点批准，那句话就不成立。
          做成结构，就没有需要读的提示。
        */}
        <label className={`switch ${editing && changed ? 'off' : ''}`}>
          <input
            type="checkbox"
            checked={p.approved}
            disabled={busy || !p.description?.['zh-CN'] || (editing && changed)}
            onChange={(e) => onWrite({ ...base, approved: e.target.checked })}
          />
          <span>
            {editing && changed
              ? '先保存描述，再批准'
              : p.approved
                ? '已批准，模型会读到上面那段'
                : '未批准，模型拿到机械生成的一句'}
          </span>
        </label>
      </Field>

      {p.manifestDescription?.['zh-CN'] && (
        <Field
          label="适配层自己的说法"
          why="这是适配层通过网络自报的。它永远不会进提示词 —— 要用就自己读一遍、抄进上面那栏、再批准。"
        >
          <ImportSuggestion
            text={p.manifestDescription}
            onImport={() => {
              setDesc(p.manifestDescription);
              setEditing(true);
            }}
          />
        </Field>
      )}
    </div>
  );
}

// ImportSuggestion 把「采纳适配层自报的描述」做成一次带确认的导入。
//
// 此前这里只有一个「抄到上面去编辑」的链接，理由是「采纳它必须是一次有意的
// 动作」。那个理由对，但把它实现成一次手工搬运是让人多干活，而不是让人多读
// 一遍 —— 真正要的是**在决定的那一刻把原文摆在眼前**。
//
// 所以：一个导入按钮，点开弹出确认框，框里逐字显示两种语言的原文。确认这个
// 动作本身就是「我读过了」，而它比手工复制更可能真的被读一遍。
//
// ⚠️ 导入之后仍然是未批准状态 —— 导入是把文字搬进编辑框，批准是另一个开关。
// 两个动作合并会让「读过」这件事重新变成一次点击。
function ImportSuggestion({ text, onImport }) {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <div className="suggestion">
        <div className="dim">{text['zh-CN']}</div>
        <button className="linkish" onClick={() => setOpen(true)}>
          导入这段描述
        </button>
      </div>
    );
  }
  return (
    <div className="suggestion confirming">
      <strong>这段文字会被放进「给模型的描述」，由你确认后才可能进提示词。</strong>
      <p className="hint small">
        它由适配层通过网络自报，来源不受这个部署控制 —— 读一遍再导入。
      </p>
      <pre className="quoted">{text['zh-CN']}</pre>
      <pre className="quoted">{text['en-US']}</pre>
      <div className="confirm-actions">
        <button
          onClick={() => {
            onImport();
            setOpen(false);
          }}
        >
          我读过了，导入
        </button>
        <button className="linkish" onClick={() => setOpen(false)}>
          取消
        </button>
      </div>
    </div>
  );
}

// BaseURLField 让地址可改，并在改之前把代价说清楚。
//
// # 为什么这里要 warn，而不是让健康状态自己去红
//
// 一个打错的地址**不会当场报错**。请求要等到下一次有人问天气才发出去，那时
// 失败的是用户的一次对话，而改地址的人早就切走了。所以代价必须在按下保存之
// 前说，而不是在之后由一个红点说。
//
// 校验仍然在后端做（link-local、内嵌凭据、非 http scheme 一律拒），因为
// 「谁在改」从来不是那道防线 —— 拒绝重定向和拒绝 link-local 才是，而这两条
// 与值从哪来无关。
function BaseURLField({ p, busy, onWrite, base }) {
  const [editing, setEditing] = useState(false);
  const [url, setUrl] = useState(p.baseUrl || '');
  const overridden = p.fileBaseUrl && p.baseUrl !== p.fileBaseUrl;

  if (!editing) {
    return (
      <Field label="地址" why="可改。改完立刻生效，这个源的健康记录会清空。">
        <div className="desc-view">
          <code>{p.baseUrl}</code>
          {overridden && (
            <div className="dim">
              文件里写的是 <code>{p.fileBaseUrl}</code>
            </div>
          )}
          <button className="linkish" onClick={() => setEditing(true)}>
            改地址
          </button>
        </div>
      </Field>
    );
  }
  return (
    <Field label="地址">
      <div className="desc-edit">
        <input value={url} onChange={(e) => setUrl(e.target.value)} spellCheck={false} />
        <Notice kind="warn" title="改完可能就连不上了">
          地址错了不会当场报错 —— 要等到下一次有人问天气才失败，而那时失败的是
          用户的一次对话。改完请用「不可用」那个标记确认它还活着。
        </Notice>
        <div className="confirm-actions">
          <button
            disabled={busy}
            onClick={() => {
              onWrite({ ...base, baseUrl: url });
              setEditing(false);
            }}
          >
            保存并切换
          </button>
          {overridden && (
            <button
              className="linkish"
              disabled={busy}
              onClick={() => {
                onWrite({ ...base, baseUrl: '' });
                setEditing(false);
              }}
            >
              恢复文件里的地址
            </button>
          )}
          <button
            className="linkish"
            onClick={() => {
              setUrl(p.baseUrl || '');
              setEditing(false);
            }}
          >
            取消
          </button>
        </div>
      </div>
    </Field>
  );
}
