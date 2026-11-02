// 對真實執行中的 hermes-console 做 API 層煙霧測試：
// 登入 → 設定 → 啟動 fixture gateway → 對話 → 核准 → 中止 → 登出。
// 用法：node api-smoke.mjs --url http://127.0.0.1:8080 --python /usr/bin/python3 --source /abs/fixture [--password x]
const args = {};
for (let i = 2; i < process.argv.length; i += 2) {
  args[process.argv[i].replace(/^--/, "")] = process.argv[i + 1];
}
const base = args.url;
const password = args.password ?? "e2e-test-password-2026";
if (!base || !args.python || !args.source) {
  console.error("需要 --url、--python、--source 參數");
  process.exit(2);
}

let cookie = "";
async function api(path, method = "GET", body) {
  const res = await fetch(base + path, {
    method,
    redirect: "manual",
    headers: {
      ...(body ? { "Content-Type": "application/json" } : {}),
      ...(cookie ? { Cookie: cookie } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const setCookie = res.headers.get("set-cookie");
  if (setCookie) cookie = setCookie.split(";")[0];
  const text = await res.text();
  let data = text;
  try {
    data = JSON.parse(text);
  } catch {}
  return { status: res.status, data };
}

function fail(step, info) {
  console.error(`FAIL [${step}]`, JSON.stringify(info).slice(0, 500));
  process.exit(1);
}
function ok(step) {
  console.log(`ok   ${step}`);
}

// --- SSE 訂閱：解析 event:/data: 行，暴露 waitFor ---
const frames = [];
const waiters = [];
const sseController = new AbortController();
function dispatchSse(block) {
  let kind = "";
  let raw = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event: ")) kind = line.slice(7).trim();
    else if (line.startsWith("data: ")) raw += line.slice(6);
  }
  if (!kind) return;
  let data = null;
  try {
    data = raw ? JSON.parse(raw) : null;
  } catch {}
  frames.push({ kind, data });
  for (const w of waiters.splice(0)) w();
}
async function startSse() {
  const res = await fetch(base + "/api/events", {
    headers: { Cookie: cookie, Accept: "text/event-stream" },
    signal: sseController.signal,
  });
  if (res.status !== 200) fail("sse", { status: res.status });
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  (async () => {
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx;
        while ((idx = buffer.indexOf("\n\n")) >= 0) {
          dispatchSse(buffer.slice(0, idx));
          buffer = buffer.slice(idx + 2);
        }
      }
    } catch {}
  })();
}
function waitFor(pred, timeoutMs, label) {
  return new Promise((resolve, reject) => {
    const check = () => {
      const hit = frames.find(pred);
      if (hit) return resolve(hit);
    };
    check();
    const timer = setTimeout(
      () => reject(new Error(`timeout waiting for ${label}`)),
      timeoutMs,
    );
    waiters.push(() => {
      const hit = frames.find(pred);
      if (hit) {
        clearTimeout(timer);
        resolve(hit);
      }
    });
  });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  // 1. 未登入存取被拒
  let r = await api("/api/me");
  if (r.status !== 401) fail("me-unauthorized", r);
  ok("未登入 API 被拒（401）");

  // 2. 錯誤密碼
  r = await api("/api/login", "POST", { password: "wrong-password" });
  if (r.status !== 401) fail("login-wrong", r);
  ok("錯誤密碼被拒");

  // 3. 正確登入
  r = await api("/api/login", "POST", { password });
  if (r.status !== 200 || !cookie) fail("login", r);
  ok("登入取得 cookie");

  // 4. 設定
  r = await api("/api/settings", "PUT", {
    python: args.python,
    source: args.source,
    workspace: "",
    home: "",
  });
  if (r.status !== 200) fail("settings", r);
  ok("連線設定已儲存");

  // 5. 啟動 gateway
  r = await api("/api/restart", "POST", {});
  if (r.status !== 200) fail("restart", r);
  let connected = false;
  for (let i = 0; i < 30; i++) {
    await sleep(1000);
    const s = await api("/api/status");
    if (s.data?.connected) {
      connected = true;
      break;
    }
  }
  if (!connected) fail("connect", "gateway 未在 30 秒內連線");
  ok("fixture gateway 已連線");

  // 6. SSE
  await startSse();
  await waitFor((f) => f.kind === "snapshot", 10_000, "snapshot");
  ok("SSE snapshot");

  // 7. 建立對話
  r = await api("/api/rpc", "POST", { method: "session.create", params: {} });
  const session = r.data?.session_id;
  if (r.status !== 200 || !session) fail("session.create", r);
  ok(`對話已建立（${session}）`);

  // 8. 第一輪 prompt → delta → approval → once → complete
  r = await api("/api/rpc", "POST", {
    method: "prompt.submit",
    params: { text: "api-smoke: 第一輪", session_id: session },
  });
  if (r.status !== 200) fail("prompt-1", r);
  await waitFor(
    (f) => f.data?.params?.type === "message.delta",
    20_000,
    "message.delta",
  );
  ok("收到串流 delta");
  const approval = await waitFor(
    (f) => f.data?.method === "approval",
    20_000,
    "approval",
  );
  ok("收到核准請求");
  r = await api("/api/respond", "POST", {
    id: approval.data.id,
    result: { choice: "once" },
  });
  if (r.status !== 200) fail("respond-once", r);
  const done1 = await waitFor(
    (f) =>
      f.data?.params?.type === "message.complete" &&
      String(f.data.params.payload?.text ?? "").includes("已收到核准回應"),
    20_000,
    "complete-1",
  );
  ok(`核准後完成回覆：${done1.data.params.payload.text.slice(0, 24)}…`);

  // 9. 第二輪 prompt → approval → interrupt → 已中止
  r = await api("/api/rpc", "POST", {
    method: "prompt.submit",
    params: { text: "api-smoke: 第二輪", session_id: session },
  });
  if (r.status !== 200) fail("prompt-2", r);
  await waitFor((f) => f.data?.method === "approval", 20_000, "approval-2");
  r = await api("/api/rpc", "POST", {
    method: "session.interrupt",
    params: { session_id: session },
  });
  if (r.status !== 200) fail("interrupt", r);
  await waitFor(
    (f) =>
      f.data?.params?.type === "message.complete" &&
      String(f.data.params.payload?.text ?? "").includes("已中止"),
    20_000,
    "interrupt-complete",
  );
  ok("中止後收到完成事件");

  // 10. 紀錄與登出
  r = await api("/api/logs");
  if (r.status !== 200 || !Array.isArray(r.data)) fail("logs", r);
  ok(`操作紀錄可讀（${r.data.length} 筆）`);
  r = await api("/api/logout", "POST", {});
  if (r.status !== 200) fail("logout", r);
  cookie = "";
  r = await api("/api/me");
  if (r.status !== 401) fail("me-after-logout", r);
  ok("登出後 API 再次被拒");

  sseController.abort();
  console.log("API SMOKE OK");
}

main().catch((e) => {
  console.error("FAIL", e.message);
  process.exit(1);
});
