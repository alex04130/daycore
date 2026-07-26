// Thin HTTP client for the Daycore API. Every request carries cookies
// (credentials: "include"); errors surface as ApiError with the server's
// stable `error` code so the UI can translate them (FRONTEND_HANDOFF §0).

export class ApiError extends Error {
  constructor(code, message, status) {
    super(message || code);
    this.code = code || 'internal';
    this.status = status || 0;
  }
}

async function handle(res) {
  let body = null;
  try { body = await res.json(); } catch (_) { /* non-JSON (e.g. 204) */ }
  if (!res.ok) {
    throw new ApiError(body && body.error, body && body.message, res.status);
  }
  // AI endpoints return 200 with an error envelope — normalize to a throw so
  // callers handle one shape.
  if (body && body.error && typeof body.error === 'string') {
    throw new ApiError(body.error, body.message, res.status);
  }
  return body;
}

function req(method, url, body) {
  return fetch(url, {
    method,
    credentials: 'include',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }).then(handle);
}

export const get = (url) => req('GET', url);
export const post = (url, body) => req('POST', url, body);
export const patch = (url, body) => req('PATCH', url, body);
export const del = (url) => req('DELETE', url);

// streamAgent consumes the SSE v2 agent stream (POST /api/ai/companion):
// each `data:` line carries one JSON frame dispatched to `on` by its `type` —
// on = {delta(text), reasoning(text), toolStart(f), toolResult(f), decision(f),
// error(f), done()}. An error frame invokes on.error then rejects with
// ApiError; a done frame resolves. Pass an AbortSignal to cancel mid-stream.
export async function streamAgent(url, body, on, signal) {
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    let payload = null;
    try { payload = await res.json(); } catch (_) {}
    throw new ApiError(payload && payload.error, payload && payload.message, res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    buf = buf.replace(/\r\n/g, '\n'); // normalize CRLF → LF (SSE spec allows either)
    let idx;
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      for (const line of frame.split('\n')) {
        if (!line.startsWith('data: ')) continue;
        let f = null;
        try { f = JSON.parse(line.slice(6)); } catch (_) { /* skip malformed frame */ }
        if (!f) continue;
        switch (f.type) {
          case 'delta': on.delta && on.delta(f.text); break;
          case 'reasoning': on.reasoning && on.reasoning(f.text); break;
          case 'tool_start': on.toolStart && on.toolStart(f); break;
          case 'tool_result': on.toolResult && on.toolResult(f); break;
          case 'decision_card': on.decision && on.decision(f); break;
          case 'error':
            on.error && on.error(f);
            reader.cancel().catch(() => {});
            throw new ApiError(f.code, f.message);
          case 'done':
            on.done && on.done();
            reader.cancel().catch(() => {});
            return;
        }
      }
    }
  }
}

// fileToBase64 reads a File and returns base64 WITHOUT the data: prefix
// (the wire format /api/ai/plan-image and extract-schedule-image expect).
export function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).replace(/^data:[^,]*,/, ''));
    r.onerror = reject;
    r.readAsDataURL(file);
  });
}

export function readFileText(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result));
    r.onerror = reject;
    r.readAsText(file);
  });
}
