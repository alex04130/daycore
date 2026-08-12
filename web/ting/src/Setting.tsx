import { useState } from 'react';
import * as api from './api';
import { backendBase, setBackendBase } from './backend';

// The first-install screen every frontend is required to ship.
//
// docs/specs/frontend-manifest.md: "强制要求每个前端实现 /setting 首次安装配置
// 界面（至少能配连哪个后端）。第三方前端写死后端地址就只能对着一个部署用，而
// 自部署是常态。"
//
// ⚠️ It PROBES before it commits. Typing an address and being told "saved" only
// to hit a blank screen is the worst version of this interaction, because the
// person cannot tell a typo from an outage from a backend that is fine but not
// a daycore. One request answers all three, and the answer is on the same
// screen as the field they would fix.
export function Setting({ onDone }: { onDone: () => void }) {
  const [value, setValue] = useState(backendBase());
  const [state, setState] = useState<'idle' | 'checking' | 'ok'>('idle');
  const [err, setErr] = useState('');

  async function check() {
    setState('checking');
    setErr('');
    const previous = backendBase();
    setBackendBase(value);
    try {
      const v = await api.probe();
      setState('ok');
      setErr('');
      void v;
    } catch (e) {
      // Put the old address back. A field that has silently changed what the
      // app talks to, while showing an error about the new one, is how somebody
      // ends up debugging the wrong deployment.
      setBackendBase(previous);
      setState('idle');
      setErr(
        api.isUnreachable(e)
          ? '连不上。检查地址、端口，以及那台机器是不是允许这个网页来源（ALLOWED_ORIGINS）。'
          : '连上了，但它不像一个 Daycore 后端：' + (e instanceof Error ? e.message : String(e)),
      );
    }
  }

  return (
    <div className="tg-app">
      <div className="tg-frame">
        <div className="tg-main">
          <div className="tg-eyebrow">
            <span>初次设置</span>
            <i className="ln" />
          </div>
          <h1 className="tg-title md">连哪个后端？</h1>
          <p className="tg-sub">
            汀 只是一个界面，你的数据在你自己的 Daycore 服务器上。
            留空就是「和这个网页同一个地址」—— 如果你是把汀和后端放在一起的，那就不用填。
          </p>
          <div className="tg-rows" style={{ marginTop: 22 }}>
            <input
              className="tg-input"
              value={value}
              onChange={(e) => {
                setValue(e.target.value);
                setState('idle');
              }}
              placeholder="https://daycore.example.com  或者留空"
              autoFocus
              spellCheck={false}
              autoCapitalize="off"
            />
          </div>
          {err && <p className="tg-note">{err}</p>}
          {state === 'ok' && <p className="tg-note">连上了。</p>}
          <div className="tg-actrow">
            {state === 'ok' ? (
              <button
                className="tg-btn pri"
                onClick={() => {
                  setBackendBase(value);
                  onDone();
                }}
              >
                就用它
              </button>
            ) : (
              <button className="tg-btn pri" disabled={state === 'checking'} onClick={check}>
                {state === 'checking' ? '连着…' : '试一下'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
