import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Confirm, Empty, Notice, Screen, useSection } from '../ui.jsx';

// The frontends attached to this deployment, their theme token spaces, and the
// builds connecting.
//
// # Why an operator ever opens this screen
//
// The handshake (`POST /api/version`) is UNAUTHENTICATED — it has to be, a
// frontend making first contact holds no credential. So anything that can reach
// the server can create a family and widen an existing one's token space. The
// answer is not a credential the first contact could not have; it is a ceiling,
// plus this screen, where somebody looks at what showed up and says which ones
// are real.
//
// Three decisions live here and nowhere else:
//
//	pin           freeze the token space. After this a build claiming the same
//	              id joins as a member and cannot widen it.
//	approve rules let that frontend's own prompt fragment reach the model.
//	              Until then the backend writes a mechanical one, so the feature
//	              works from day one and the injection surface is zero.
//	move a build  the reason familyId is overridable — a new platform's build
//	              joins an existing family and inherits its themes.
//
// ⚠️ Every one of these was readable and honoured by the server one batch before
// anything could set it. A switch that is read and has no hand on it is the same
// defect as a permission nothing checks, seen from the other side.
export function Frontends({ onUnauthorized, principal }) {
  const load = useCallback(() => api.getFrontends(), []);
  const { reload, ...state } = useSection(load, { onUnauthorized });
  const canManage = principal?.root || principal?.permissions?.includes('frontends.manage');

  return (
    <Screen
      title="前端"
      sub="哪些前端接在这个部署上、它们各自认得哪些主题变量。第三方前端与多端是一等场景，所以它们报上来的东西按外部数据对待 —— 这一页就是「看一眼，然后说哪些是真的」。"
      state={state}
    >
      {state.data && (
        <>
          <Notice kind="info" title="握手是不带凭据的">
            前端第一次连上来时手里还没有任何凭据，所以 <code>POST /api/version</code> 必须是开放的。
            代价是：能连到这台服务器的东西都能建一个 family、也能给一个没钉住的 family
            加新变量。上限是 <b>{state.data.limits?.maxFamilies ?? 64}</b> 个 family，
            剩下的靠你在这里钉。
          </Notice>

          {state.data.families.length === 0 && (
            <Empty>还没有前端握过手。现役前端不握手，它按内置的兜底 token 空间走。</Empty>
          )}

          {state.data.families.map((f) => (
            <Family
              key={f.id}
              family={f}
              kinds={state.data.kinds}
              families={state.data.families}
              limits={state.data.limits}
              isFallback={f.id === state.data.fallbackFamilyId}
              canManage={canManage}
              onChanged={reload}
            />
          ))}

          {state.data.orphans?.length > 0 && (
            <div className="row">
              <div className="row-main">
                <div className="row-title">
                  <strong>没有归属的 build</strong>
                  <span className="pill warn">{state.data.orphans.length}</span>
                </div>
                <div className="sub">
                  它们指着的 family 已经不在了 —— 多半是有人在还有东西连着的时候删了它。
                  留在这里是因为它们是那件事仅有的痕迹：藏起来只会让人对着一个「前端不见了」发呆。
                  把它们挪进一个存在的 family 就能继续用。
                </div>
                {state.data.orphans.map((b) => (
                  <Build
                    key={b.buildHash}
                    build={b}
                    families={state.data.families}
                    canManage={canManage}
                    onChanged={reload}
                  />
                ))}
              </div>
            </div>
          )}

          <Notice kind="info" title="兜底 token 空间">
            没握过手的前端（包括现役这个）按 <code>{state.data.fallbackFamilyId}</code> 判定，
            那份是内置的、改不了。这就是改造期间现役前端行为不变的原因。
          </Notice>
        </>
      )}
    </Screen>
  );
}

function Family({ family: f, kinds, families, limits, isFallback, canManage, onChanged }) {
  const [err, setErr] = useState('');
  const [showTokens, setShowTokens] = useState(false);
  const set = (patch) => api.setFrontendFamily(f.id, patch).then(onChanged, (e) => setErr(e.message));

  return (
    <div className="row">
      <div className="row-main">
        <div className="row-title">
          <strong>{f.displayName || f.id}</strong>
          <code className="muted">{f.id}</code>
          {f.pinned && <span className="pill">已钉住</span>}
          {f.rulesAccepted && <span className="pill warn">主题规则已批准 · 进模型</span>}
          {isFallback && <span className="pill">兜底</span>}
        </div>

        <div className="usage-line">
          <span title="一个 family 的 token 空间是它所有 build 声明过的并集，从不取交集 —— 取交集会让「多接一个端」变成「从另一个端的主题里删变量」。">
            <b>{f.tokens.length}</b> 个主题变量
          </span>
          <span>
            <b>{f.builds.length}</b> 个 build 在连
          </span>
          <span>建于 <b>{f.createdAt ? f.createdAt.slice(0, 10) : '—'}</b></span>
          <button className="linkish" onClick={() => setShowTokens((v) => !v)}>
            {showTokens ? '收起变量' : '看变量'}
          </button>
        </div>

        {showTokens && <TokenTable tokens={f.tokens} kinds={kinds} />}

        {f.rules && (
          <div className="field">
            <div className="field-label">这个前端自己写的主题规则</div>
            <div className="field-value">
              <pre className="key-value">{f.rules}</pre>
            </div>
          </div>
        )}

        <Backfill family={f} limits={limits} canManage={canManage} onChanged={onChanged} />

        {err && <Notice kind="error">{err}</Notice>}
      </div>

      {canManage && (
        <div className="row-actions">
          {f.pinned ? (
            <Confirm
              word={f.id}
              label="取消钉住"
              danger="取消后，任何能连到这台服务器的东西都可以给它加新的主题变量。"
              onConfirm={() => set({ pinned: false })}
            />
          ) : (
            <button onClick={() => set({ pinned: true })} title="冻结 token 空间：之后声明同一个 id 的 build 只能作为成员加入，不能再加变量。">
              钉住
            </button>
          )}

          {f.rules &&
            (f.rulesAccepted ? (
              <Confirm
                word={f.id}
                label="撤回批准"
                danger="撤回后它的规则不再进模型，主题生成回到后端按变量表机械生成的那份。"
                onConfirm={() => set({ rulesAccepted: false })}
              />
            ) : (
              <Confirm
                word={f.id}
                label="批准它的主题规则"
                danger={
                  '⚠️ 批准之后，上面那段由前端上报的文字会进到生成主题的提示词里。' +
                  '第三方前端是一等场景，所以那是别人写的文字 —— 先读一遍再批。' +
                  '不批也不影响功能：后端会按变量表机械生成一份。'
                }
                onConfirm={() => set({ rulesAccepted: true })}
              />
            ))}

          <Confirm
            word={f.id}
            label="删除"
            danger={
              f.builds.length > 0
                ? '还有 build 指着它，服务端会拒。先把那些 build 挪走。'
                : '⚠️ 不会删掉存在它名下的主题 —— 那些是用户数据，可能有几千条。' +
                  '删掉 family 之后它们还在库里，但没有任何界面能再看到它们。'
            }
            onConfirm={() => api.deleteFrontendFamily(f.id).then(onChanged, (e) => setErr(e.message))}
          />
        </div>
      )}

      {f.builds.length > 0 && (
        <div className="row-main">
          {f.builds.map((b) => (
            <Build key={b.buildHash} build={b} families={families} canManage={canManage} onChanged={onChanged} />
          ))}
        </div>
      )}
    </div>
  );
}

// ⚠️ `kind` is shown beside every token on purpose. An operator reading
// `--glass-alpha: ratio` has no way to know what a ratio may be, and the kinds
// are DATA — a deployment can have kinds this console has never heard of.
function TokenTable({ tokens, kinds }) {
  const byName = new Map(kinds.map((k) => [k.name, k]));
  return (
    <table className="table">
      <thead>
        <tr>
          <th>变量</th>
          <th>类型</th>
          <th>允许的形状</th>
          <th>说明</th>
        </tr>
      </thead>
      <tbody>
        {tokens.map((t) => {
          const k = byName.get(t.kind);
          return (
            <tr key={t.name}>
              <td><code>{t.name}</code></td>
              <td><code>{t.kind}</code></td>
              <td className="muted">
                {k ? <code>{k.pattern}</code> : <span title="组合类型（one-of / list-of / nullable），每一片最终还是落到一个基础类型上">组合</span>}
              </td>
              <td className="sub">{t.description || k?.description || ''}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// Moving a build is what makes a new platform's build inherit an existing
// family's themes.
//
// ⚠️ The server deliberately does not let the next handshake undo it: a build
// declares at every startup the family it was compiled with, and the operator's
// assignment wins. Otherwise what somebody did in this console would be
// silently reverted by a restart.
function Build({ build: b, families, canManage, onChanged }) {
  const [moving, setMoving] = useState(false);
  const [target, setTarget] = useState('');
  const [err, setErr] = useState('');

  const others = families.filter((f) => f.id !== b.familyId);

  return (
    <div className="usage-line">
      <span>
        <b>{b.displayName || b.buildHash}</b> {b.version && <code className="muted">{b.version}</code>}
      </span>
      <span title="按五分钟的粒度记，不是每次握手都写。">
        上次连 {b.lastSeenAt ? b.lastSeenAt.slice(0, 16).replace('T', ' ') : '—'}
      </span>
      {b.minApi > 0 && <span title="这个 build 声明它至少需要的 API 版本。">需要 API ≥ {b.minApi}</span>}
      {canManage && others.length > 0 && !moving && (
        <button className="linkish" onClick={() => setMoving(true)}>
          换 family
        </button>
      )}
      {moving && (
        <>
          <select value={target} onChange={(e) => setTarget(e.target.value)}>
            <option value="">选一个…</option>
            {others.map((f) => (
              <option key={f.id} value={f.id}>
                {f.displayName || f.id}
              </option>
            ))}
          </select>
          <button
            disabled={!target}
            onClick={() =>
              api.setBuildFamily(b.buildHash, target).then(
                () => {
                  setMoving(false);
                  onChanged();
                },
                (e) => setErr(e.message),
              )
            }
          >
            挪过去
          </button>
          <button className="linkish" onClick={() => setMoving(false)}>
            取消
          </button>
        </>
      )}
      {err && <Notice kind="error">{err}</Notice>}
    </div>
  );
}

// Filling in the tokens a family's stored themes are missing.
//
// # ⚠️ The price is on the screen before the button is
//
// One model call per theme. A family with two thousand themes is two thousand
// calls, and the widening that made them incomplete is a routine deploy — so
// this is never automatic, and the number is loaded on demand rather than with
// the family list (counting it for every family on every console load would be
// slow in exactly the deployment where the number matters).
//
// `capped` means the count stopped at its ceiling, and the screen says "至少"
// rather than pretending to be exact. That is the honest sentence for the only
// question being asked here, which is "is this a lot".
function Backfill({ family: f, limits, canManage, onChanged }) {
  const [price, setPrice] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const running = !!f.backfillRequestedAt;

  const look = () => {
    setBusy(true);
    setErr('');
    api.getBackfillPrice(f.id).then(
      (p) => { setPrice(p); setBusy(false); },
      (e) => { setErr(e.message); setBusy(false); },
    );
  };

  if (running) {
    return (
      <Notice kind="info" title="补算进行中">
        运维已经点过了。leader 每分钟补一批（一批 {limits?.backfillPerSweep} 套），补完会自己停。
        主题多的话要跑很久 —— 这是对的速度，没有人在同步等它。
        {canManage && (
          <div className="row-actions">
            <Confirm
              word={f.id}
              label="停掉"
              danger={
                '⚠️ 这是停，不是撤销。已经补好的主题**保持补好的样子** —— 主题没有撤销这回事，' +
                '这也是补算只往里加、从不覆盖已有取值的原因。'
              }
              onConfirm={() => api.stopBackfill(f.id).then(onChanged, (e) => setErr(e.message))}
            />
          </div>
        )}
        {err && <Notice kind="error">{err}</Notice>}
      </Notice>
    );
  }

  return (
    <div className="usage-line">
      {price === null ? (
        <button className="linkish" disabled={busy} onClick={look}>
          {busy ? '数着…' : '看看有多少主题缺变量'}
        </button>
      ) : price.themes === 0 ? (
        <span className="muted">这个 family 的主题都是全的，不用补。</span>
      ) : (
        <>
          <span>
            有 <b>{price.capped ? `至少 ${price.themes}` : price.themes}</b> 套主题缺变量
          </span>
          <span className="muted" title="补算是一套主题一次模型调用。这是要花的钱，所以永远不会自动跑。">
            补一次 = {price.capped ? '至少 ' : ''}{price.themes} 次模型调用
          </span>
          {canManage && (
            <Confirm
              word={f.id}
              label="开始补算"
              danger={
                `会对 ${price.capped ? '至少 ' : ''}${price.themes} 套主题各做一次模型调用，按用户已有的配色补上缺的变量。` +
                '只往里加，不动用户自己选的取值。跑起来之后可以停，但停不回已经补好的那些。'
              }
              onConfirm={() => api.startBackfill(f.id).then(onChanged, (e) => setErr(e.message))}
            />
          )}
          <button className="linkish" onClick={look}>重新数</button>
        </>
      )}
      {err && <Notice kind="error">{err}</Notice>}
    </div>
  );
}
