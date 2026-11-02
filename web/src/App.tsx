import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import Markdown from "react-markdown";
import SecuritySettings from "./SecuritySettings";
import WorkspaceFiles from "./WorkspaceFiles";
import MaintenanceSettings from "./MaintenanceSettings";
import ConversationSearch from "./ConversationSearch";
import { notifyFrame } from "./notifications";
import { downloadConversation } from "./conversationExport";
import {
  ArrowDown,
  Archive,
  Trash2,
  ArrowUp,
  ArrowUpRight,
  Activity,
  Check,
  ChevronDown,
  Circle,
  Command,
  Folder,
  GitBranch,
  Hexagon,
  LogOut,
  Menu,
  Merge,
  MessageSquare,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Settings2,
  ShieldCheck,
  Sparkles,
  Square,
  Terminal,
  X,
} from "lucide-react";
import {
  applyFrame,
  historyMessages,
  initialChat,
  type Chat,
  type Frame,
} from "./state";

type Settings = {
  python: string;
  source: string;
  workspace: string;
  home: string;
};
type Status = { connected: boolean; state: string; error?: string };
type Session = {
  id: string;
  title?: string;
  model?: string;
  started_at?: number;
};
type Entry = { id: number; time: string; kind: string; detail: string };
type UsageInfo = {
  items: {
    time: string;
    session_id: string;
    calls: number;
    input: number;
    output: number;
    total: number;
  }[];
  totals: { calls: number; input: number; output: number; total: number };
};
type InstallStatus = {
  running: boolean;
  done: boolean;
  error?: string;
  steps: string[];
  log: string[];
  python?: string;
  source?: string;
};
class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
async function api<T = Record<string, any>>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api" + path, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok)
    throw new APIError(data.error ?? "請求失敗", response.status);
  return data;
}
const rpc = (method: string, params: Record<string, unknown> = {}) =>
  api("/rpc", "POST", { method, params });
const emptySettings: Settings = {
  python: "python",
  source: "",
  workspace: "",
  home: "",
};
const stateNames: Record<string, string> = {
  ready: "已連線",
  starting: "連線中",
  stopped: "未連線",
  error: "連線失敗",
};

function Brand() {
  return (
    <div className="brand">
      <div className="brand-icon">
        <Hexagon size={25} />
        <span>h</span>
      </div>
      <div>
        hermes<span className="brand-caption">AGENT CONSOLE</span>
      </div>
    </div>
  );
}
export default function App() {
  const [auth, setAuth] = useState<boolean | null>(null),
    [password, setPassword] = useState(""),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const [page, setPage] = useState<"chat" | "settings" | "logs">("chat"),
    [mobile, setMobile] = useState(false),
    [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<Status>({
      connected: false,
      state: "stopped",
    }),
    [settings, setSettings] = useState<Settings>(emptySettings);
  const [sessions, setSessions] = useState<Session[]>([]),
    [search, setSearch] = useState(""),
    [chat, setChat] = useState<Chat>(initialChat("")),
    [text, setText] = useState("");
  const [logs, setLogs] = useState<Entry[]>([]),
    [events, setEvents] = useState<Entry[]>([]),
    [eventConnected, setEventConnected] = useState(false);
  const [usage, setUsage] = useState<UsageInfo | null>(null),
    [diag, setDiag] = useState("");
  const [model, setModel] = useState(""),
    [provider, setProvider] = useState(""),
    [modelOptions, setModelOptions] = useState<unknown>(null),
    [modelOpen, setModelOpen] = useState(false);
  const [configKey, setConfigKey] = useState("prompt"),
    [configValue, setConfigValue] = useState("");
  const [install, setInstall] = useState({ python: "", dir: "", ref: "main" }),
    [installSt, setInstallSt] = useState<InstallStatus | null>(null);
  const latest = useRef(chat),
    statusRef = useRef(status),
    end = useRef<HTMLDivElement>(null),
    loadID = useRef(0),
    hydrating = useRef(false),
    buffer = useRef<Frame[]>([]);
  latest.current = chat;
  statusRef.current = status;
  const startInstall = () =>
    run(async () => {
      await api("/install/start", "POST", {
        python: install.python,
        targetDir: install.dir,
        ref: install.ref,
      });
      setInstallSt({ running: true, done: false, steps: [], log: [] });
    });
  // 安裝進行中輪詢狀態；完成且成功時自動把 venv Python 與原始碼目錄填入連線設定。
  useEffect(() => {
    if (!installSt?.running) return;
    const timer = setTimeout(async () => {
      try {
        setInstallSt(await api<InstallStatus>("/install/status"));
      } catch (e) {
        setInstallSt({
          running: false,
          done: true,
          error: e instanceof Error ? e.message : "無法取得安裝進度",
          steps: [],
          log: [],
        });
      }
    }, 700);
    return () => clearTimeout(timer);
  }, [installSt]);
  useEffect(() => {
    if (
      installSt?.done &&
      !installSt.error &&
      (installSt.python || installSt.source)
    ) {
      setSettings((s) => ({
        ...s,
        python: installSt.python || s.python,
        source: installSt.source || s.source,
      }));
      setNotice("Hermes 安裝完成，已填入連線設定");
    }
  }, [installSt]);
  const report = useCallback((e: unknown) => {
    if (e instanceof APIError && e.status === 401) {
      setAuth(false);
      setChat(initialChat(""));
    }
    setError(e instanceof Error ? e.message : String(e));
  }, []);
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
    } catch (e) {
      report(e);
    } finally {
      setBusy(false);
    }
  };
  const refreshSessions = useCallback(async () => {
    const data = await rpc("session.list", { limit: 100 });
    setSessions(data.sessions ?? []);
  }, []);
  const hydrate = useCallback(async (method: string, id: string) => {
    const version = ++loadID.current;
    hydrating.current = true;
    buffer.current = [];
    try {
      const data = await rpc(method, { session_id: id });
      if (version !== loadID.current) return;
      const sid = String(data.session_id ?? id),
        pending = [...buffer.current];
      setChat((previous) => {
        let next = {
          ...initialChat(sid),
          requests: previous.requests,
          messages: historyMessages(data.messages ?? []),
          running: !!(data.running ?? data.inflight?.streaming),
        };
        if (data.inflight?.assistant) {
          const value = data.inflight.assistant;
          const fragment =
            typeof value === "string" ? value : String(value.text ?? "");
          if (fragment)
            next.messages.push({ role: "assistant", text: fragment });
        }
        for (const frame of [...(data.open_requests ?? []), ...pending])
          next = applyFrame(next, frame);
        return next;
      });
      if (data.info?.model) setModel(data.info.model);
      if (data.info?.provider) setProvider(data.info.provider);
    } finally {
      if (version === loadID.current) {
        hydrating.current = false;
        buffer.current = [];
      }
    }
  }, []);
  useEffect(() => {
    let alive = true;
    api("/me")
      .then(() => {
        if (alive) setAuth(true);
      })
      .catch((e) => {
        if (alive) {
          setAuth(false);
          if (!(e instanceof APIError && e.status === 401)) report(e);
        }
      });
    return () => {
      alive = false;
    };
  }, [report]);
  useEffect(() => {
    if (!auth) return;
    let alive = true;
    Promise.all([
      api<Status>("/status"),
      api<Settings>("/settings"),
      api<Entry[]>("/logs"),
    ])
      .then(([s, c, l]) => {
        if (!alive) return;
        setStatus(s);
        setSettings(c);
        setLogs(l);
        if (s.connected) refreshSessions().catch(report);
      })
      .catch(report);
    const source = new EventSource("/api/events", { withCredentials: true });
    source.addEventListener("snapshot", (event) => {
      const data = JSON.parse((event as MessageEvent).data);
      setEventConnected(true);
      setStatus(data.status);
      setChat((old) => ({ ...old, requests: data.requests ?? [] }));
      if (data.status.connected) {
        refreshSessions().catch(report);
        if (latest.current.session)
          hydrate("session.activate", latest.current.session).catch(report);
      }
    });
    source.addEventListener("frame", (event) => {
      const frame: Frame = JSON.parse((event as MessageEvent).data);
      notifyFrame(frame);
      if (frame.method === "console.status") {
        const next = frame.params as Status;
        setStatus(next);
        if (next.connected && !statusRef.current.connected)
          refreshSessions().catch(report);
        if (!next.connected) {
          ++loadID.current;
          hydrating.current = false;
          buffer.current = [];
          setChat((old) => ({
            ...initialChat(""),
            requests: [],
            messages: old.messages,
          }));
          setSessions([]);
        }
      } else {
        if (hydrating.current) buffer.current.push(frame);
        setChat((old) => applyFrame(old, frame));
      }
      const kind = String(frame.params?.type ?? frame.method);
      if (
        kind === "message.complete" &&
        typeof frame.params?.session_id === "string"
      )
        rpc("session.history", { session_id: frame.params.session_id }).catch(
          report,
        );
      if (kind === "error" || kind === "session.error")
        setError(
          String(
            frame.params?.payload?.message ??
              frame.params?.payload?.error ??
              "Hermes 執行失敗",
          ),
        );
      if (kind !== "message.delta" && kind !== "reasoning.delta")
        setEvents((old) =>
          [
            {
              id: Date.now() + Math.random(),
              time: new Date().toISOString(),
              kind: "event",
              detail: kind,
            },
            ...old,
          ].slice(0, 100),
        );
    });
    source.addEventListener("error", () => {
      setEventConnected(false);
      api("/me").catch(report);
    });
    source.addEventListener("expired", () => {
      setAuth(false);
      source.close();
    });
    return () => {
      alive = false;
      source.close();
    };
  }, [auth, hydrate, refreshSessions, report]);
  useEffect(() => {
    end.current?.scrollIntoView?.({ behavior: "smooth", block: "end" });
  }, [chat.messages.length, chat.messages.at(-1)?.text]);
  const create = () =>
    run(async () => {
      ++loadID.current;
      hydrating.current = false;
      buffer.current = [];
      const result = await rpc("session.create", {
        ...(model ? { model } : {}),
        ...(provider ? { provider } : {}),
        idempotency_key: crypto.randomUUID(),
      });
      setChat((old) => ({
        ...initialChat(result.session_id),
        requests: old.requests,
        messages: historyMessages(result.messages ?? []),
      }));
      setPage("chat");
      setMobile(false);
      setText("");
      await refreshSessions();
    });
  const send = (e?: FormEvent) => {
    e?.preventDefault();
    if (!text.trim() || !chat.session || chat.running) return;
    const message = text.trim(),
      sid = chat.session,
      optimistic = { role: "user" as const, text: message };
    setText("");
    setChat((old) => ({
      ...old,
      messages: [...old.messages, optimistic],
      running: true,
      tools: [],
    }));
    run(async () => {
      try {
        await rpc("prompt.submit", { session_id: sid, text: message });
      } catch (e) {
        setText(message);
        setChat((old) =>
          old.session === sid
            ? {
                ...old,
                messages: old.messages.filter((item) => item !== optimistic),
                running: false,
              }
            : old,
        );
        throw e;
      }
    });
  };
  const respond = async (frame: Frame, result: unknown) => {
    await api("/respond", "POST", { id: frame.id, result });
    setChat((old) => ({
      ...old,
      requests: old.requests.filter((r) => r.id !== frame.id),
    }));
  };
  const nav = (next: typeof page) => {
    setPage(next);
    setMobile(false);
    setError("");
    setNotice("");
    if (next === "logs") {
      api<Entry[]>("/logs").then(setLogs).catch(report);
      api<UsageInfo>("/usage")
        .then(setUsage)
        .catch(() => setUsage(null));
      api<{ stderr: string }>("/diag")
        .then((d) => setDiag(d.stderr ?? ""))
        .catch(() => setDiag(""));
    }
  };
  const login = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      await api("/login", "POST", { password });
      setPassword("");
      setAuth(true);
    });
  };
  if (auth === null)
    return (
      <div className="loading-screen">
        <Brand />
        <p>正在開啟工作台…</p>
      </div>
    );
  if (!auth)
    return (
      <div className="login-shell">
        <div className="login-art">
          <span className="eyebrow">YOUR IDEAS. AN AGENT. NO LIMITS.</span>
          <h1>
            讓想法，
            <br />
            開始執行。
          </h1>
          <div className="orbital">
            <Hexagon />
            <span>h</span>
          </div>
          <p>
            一個安靜、專注的地方，
            <br />
            與你的 Hermes 一起完成工作。
          </p>
          <span className="art-foot">HERMES / PERSONAL AGENT WORKSPACE</span>
        </div>
        <main className="login-main">
          <Brand />
          <form onSubmit={login} className="login-form">
            <span className="eyebrow">WELCOME BACK</span>
            <h2>進入你的工作台</h2>
            <p>使用伺服器設定的管理密碼登入。</p>
            <label>
              管理密碼
              <input
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoFocus
              />
            </label>
            {error && (
              <div className="error" role="alert">
                {error}
              </div>
            )}
            <button className="primary" disabled={busy}>
              登入控制台 <ArrowUpRight size={17} />
            </button>
            <small>
              <ShieldCheck size={14} /> 僅你與你的 Agent
            </small>
          </form>
          <span className="login-footer">
            SELF-HOSTED. BUILT FOR YOUR FLOW.
          </span>
        </main>
      </div>
    );
  const title =
    page === "settings"
      ? "連線設定"
      : page === "logs"
        ? "執行紀錄"
        : chat.session
          ? (sessions.find((s) => s.id === chat.session)?.title ??
            "進行中的對話")
          : "工作台";
  return (
    <div className="app-shell">
      {mobile && (
        <button
          className="sidebar-backdrop"
          aria-label="關閉側欄"
          onClick={() => setMobile(false)}
        />
      )}
      <aside className={`sidebar ${mobile ? "mobile-open" : ""}`}>
        <Brand />
        <button
          className="new-chat"
          onClick={create}
          disabled={!status.connected || busy}
        >
          <Plus size={17} /> 新增對話 <span aria-hidden="true">↗</span>
        </button>
        <div className="sidebar-section">
          <span>WORKSPACE</span>
          <span>01</span>
        </div>
        <button
          className={`nav-item ${page === "chat" ? "active" : ""}`}
          onClick={() => nav("chat")}
        >
          <MessageSquare size={17} /> 對話工作台 <span className="nav-dot" />
        </button>
        <button
          className={`nav-item ${page === "logs" ? "active" : ""}`}
          onClick={() => nav("logs")}
        >
          <Activity size={17} /> 執行紀錄
        </button>
        <div className="sidebar-section history-label">
          <span>最近對話</span>
          <button
            aria-label="重新整理對話"
            disabled={!status.connected || busy}
            onClick={() => run(refreshSessions)}
          >
            <RefreshCw size={12} />
          </button>
        </div>
        <input
          className="session-search"
          aria-label="搜尋對話"
          placeholder="搜尋對話…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <ConversationSearch
          request={api}
          open={async (id) => {
            await hydrate("session.resume", id);
            setPage("chat");
            setMobile(false);
          }}
        />
        <button
          onClick={() =>
            run(async () => {
              if (typeof Notification === "undefined")
                throw new Error("此瀏覽器不支援桌面通知");
              const permission = await Notification.requestPermission();
              setNotice(
                permission === "granted"
                  ? "已啟用背景回覆與核准通知"
                  : "瀏覽器未允許通知",
              );
            })
          }
        >
          啟用桌面通知
        </button>
        <div className="session-list">
          {sessions
            .filter((s) =>
              (s.title ?? s.id).toLowerCase().includes(search.toLowerCase()),
            )
            .map((s) => (
              <div
                key={s.id}
                className={`session-row ${chat.session === s.id ? "selected" : ""}`}
              >
                <button
                  className={`session-item ${chat.session === s.id ? "selected" : ""}`}
                  onClick={() =>
                    run(async () => {
                      await hydrate("session.resume", s.id);
                      setPage("chat");
                      setMobile(false);
                    })
                  }
                  disabled={busy}
                >
                  <MessageSquare size={14} />
                  <span>{s.title || "未命名對話"}</span>
                </button>
                <span className="item-actions">
                  <button
                    aria-label={`歸檔 ${s.title ?? s.id}`}
                    title="歸檔"
                    disabled={busy || !status.connected}
                    onClick={() =>
                      run(async () => {
                        await rpc("session.archive", {
                          session_id: s.id,
                          archived: true,
                        });
                        setNotice("對話已歸檔");
                        await refreshSessions();
                      })
                    }
                  >
                    <Archive size={13} />
                  </button>
                  <button
                    aria-label={`刪除 ${s.title ?? s.id}`}
                    title="永久刪除（需先關閉執行中的對話）"
                    disabled={busy || !status.connected}
                    onClick={() => {
                      if (
                        !window.confirm(
                          "永久刪除此對話與逐字稿？此操作無法復原。執行中的對話需先關閉。",
                        )
                      )
                        return;
                      run(async () => {
                        await rpc("session.delete", { session_id: s.id });
                        setNotice("對話已永久刪除");
                        await refreshSessions();
                      });
                    }}
                  >
                    <Trash2 size={13} />
                  </button>
                  <button
                    aria-label={`重新命名 ${s.title ?? s.id}`}
                    title="重新命名"
                    disabled={busy || !status.connected}
                    onClick={() => {
                      const t = window.prompt("重新命名對話", s.title ?? "");
                      if (!t) return;
                      run(async () => {
                        await rpc("session.title", {
                          session_id: s.id,
                          title: t,
                        });
                        setNotice("對話已重新命名");
                        await refreshSessions();
                      });
                    }}
                  >
                    <Pencil size={13} />
                  </button>
                  <button
                    aria-label={`關閉對話 ${s.title ?? s.id}`}
                    title="關閉對話"
                    disabled={busy || !status.connected}
                    onClick={() =>
                      run(async () => {
                        await rpc("session.close", { session_id: s.id });
                        if (chat.session === s.id) setChat(initialChat(""));
                        setNotice("對話已關閉");
                        await refreshSessions();
                      })
                    }
                  >
                    <X size={13} />
                  </button>
                </span>
              </div>
            ))}
          {!sessions.length && (
            <p className="sidebar-empty">
              {status.connected ? "你的對話會出現在這裡" : "連線後即可讀取對話"}
            </p>
          )}
        </div>
        <div className="sidebar-bottom">
          <button
            className={`nav-item ${page === "settings" ? "active" : ""}`}
            onClick={() => nav("settings")}
          >
            <Settings2 size={17} /> 連線設定
          </button>
          <div className="account">
            <div className="avatar">Y</div>
            <div>
              個人工作空間<small>Self-hosted · Hermes</small>
            </div>
            <button
              aria-label="登出"
              title="登出"
              onClick={() =>
                run(async () => {
                  await api("/logout", "POST", {});
                  setAuth(false);
                  setChat(initialChat(""));
                })
              }
            >
              <LogOut size={16} />
            </button>
          </div>
        </div>
      </aside>
      <main className="main-area">
        <header className="topbar">
          <div className="breadcrumbs">
            <button
              className="mobile-menu"
              aria-label="開啟側欄"
              onClick={() => setMobile(true)}
            >
              <Menu size={19} />
            </button>
            <span>個人空間</span>
            <span className="slash">/</span>
            <strong>{title}</strong>
          </div>
          <div className="top-status">
            <span
              className={`status-dot ${status.connected ? "online" : ""}`}
            />
            {stateNames[status.state] ?? status.state}
            <span className="top-divider" />
            <span className="version">HERMES</span>
          </div>
        </header>
        <div className="notification-area">
          {error && (
            <div className="error" role="alert">
              <span>{error}</span>
              <button aria-label="關閉錯誤" onClick={() => setError("")}>
                <X size={15} />
              </button>
            </div>
          )}
          {notice && (
            <div className="notice" role="status">
              <Check size={15} />
              {notice}
            </div>
          )}
          {auth && status.connected && !eventConnected && (
            <div className="stream-warning">
              事件連線尚未就緒；恢復連線後會重新讀取對話。
            </div>
          )}
        </div>
        {page === "chat" && (
          <>
            <div className="chat-toolbar">
              <span>
                <span className="tiny-square" /> HERMES AGENT
              </span>
              <div className="toolbar-actions">
                <button
                  disabled={!chat.messages.length}
                  onClick={() =>
                    downloadConversation(chat.session, chat.messages, "md")
                  }
                >
                  匯出 Markdown
                </button>
                <button
                  disabled={!chat.messages.length}
                  onClick={() =>
                    downloadConversation(chat.session, chat.messages, "json")
                  }
                >
                  匯出 JSON
                </button>
                <button
                  onClick={() => setModelOpen(!modelOpen)}
                  disabled={!status.connected}
                >
                  <Sparkles size={13} />
                  {model || "預設模型"}
                  <ChevronDown size={13} />
                </button>
                {chat.session && (
                  <>
                    <button
                      aria-label="重新讀取目前對話"
                      disabled={busy}
                      onClick={() =>
                        run(async () => {
                          await hydrate("session.activate", chat.session);
                        })
                      }
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      aria-label="壓縮對話"
                      title="壓縮對話（摘要化長對話）"
                      disabled={busy}
                      onClick={() =>
                        run(async () => {
                          await rpc("session.compress", {
                            session_id: chat.session,
                          });
                          setNotice("已送出壓縮請求，完成後重新讀取");
                          await refreshSessions();
                          await hydrate("session.resume", chat.session);
                        })
                      }
                    >
                      <Merge size={14} />
                    </button>
                    <button
                      aria-label="建立分支"
                      title="以此對話建立分支"
                      disabled={busy}
                      onClick={() =>
                        run(async () => {
                          const data = await rpc("session.branch_whole", {
                            session_id: chat.session,
                          });
                          setNotice("已建立分支");
                          await refreshSessions();
                          const sid = data?.session_id;
                          if (sid)
                            await hydrate("session.activate", String(sid));
                        })
                      }
                    >
                      <GitBranch size={14} />
                    </button>
                  </>
                )}
              </div>
            </div>
            {modelOpen && (
              <section className="model-panel">
                <div>
                  <h3>模型選擇</h3>
                  <p>指定下一個對話的模型，或套用至目前對話。</p>
                </div>
                <label>
                  Provider
                  <input
                    value={provider}
                    placeholder="例如 openrouter"
                    onChange={(e) => setProvider(e.target.value)}
                  />
                </label>
                <label>
                  模型 ID
                  <input
                    value={model}
                    placeholder="留空使用 Hermes 預設"
                    onChange={(e) => setModel(e.target.value)}
                  />
                </label>
                <div className="button-row">
                  <button
                    disabled={busy}
                    onClick={() =>
                      run(async () =>
                        setModelOptions(await rpc("model.options")),
                      )
                    }
                  >
                    查詢可用模型
                  </button>
                  <button
                    className="primary"
                    disabled={busy || !chat.session || !model.trim()}
                    onClick={() =>
                      run(async () => {
                        const result = await rpc("config.set", {
                          key: "model",
                          value:
                            model + (provider ? " --provider " + provider : ""),
                          session_id: chat.session,
                        });
                        if (result.confirm_required)
                          throw new Error(
                            "此模型需要額外費用確認，請先於 Hermes CLI 完成選擇。",
                          );
                        setNotice("模型切換已提交");
                        setModelOpen(false);
                      })
                    }
                  >
                    套用目前對話
                  </button>
                </div>
                {modelOptions !== null && (
                  <details open>
                    <summary>Hermes 回傳的可用模型</summary>
                    <pre>{JSON.stringify(modelOptions, null, 2)}</pre>
                  </details>
                )}
              </section>
            )}
            <div className="conversation">
              {!chat.messages.length && (
                <div className="welcome">
                  <div className="welcome-mark">
                    <Hexagon size={29} />
                  </div>
                  <span className="eyebrow">A LITTLE SPACE FOR BIG IDEAS</span>
                  <h1>今天，一起完成什麼？</h1>
                  <p>
                    從一個想法開始。讓 Hermes 幫你探索、建立，
                    <br className="desktop-break" />
                    並把待辦變成已完成。
                  </p>
                  <div className="suggestions">
                    {[
                      {
                        icon: <Terminal size={19} />,
                        title: "理解一個專案",
                        text: "請先閱讀目前工作目錄，說明專案結構與執行方式。",
                      },
                      {
                        icon: <Command size={19} />,
                        title: "解決一個問題",
                        text: "幫我分析目前專案可能的問題，先提出調查步驟。",
                      },
                      {
                        icon: <Folder size={19} />,
                        title: "整理工作計畫",
                        text: "請先了解工作目錄，再協助我整理下一步工作計畫。",
                      },
                    ].map((s) => (
                      <button
                        key={s.title}
                        disabled={!status.connected}
                        onClick={async () => {
                          if (!chat.session) await create();
                          setText(s.text);
                        }}
                      >
                        {s.icon}
                        <strong>{s.title}</strong>
                        <span>{s.text}</span>
                        <ArrowUpRight size={15} />
                      </button>
                    ))}
                  </div>
                  {!status.connected && (
                    <div className="setup-callout">
                      <Circle size={13} />
                      <span>先連接 Hermes，開始你的第一個對話。</span>
                      <button onClick={() => nav("settings")}>
                        設定連線 <ArrowUpRight size={13} />
                      </button>
                    </div>
                  )}
                  {status.connected && !chat.session && (
                    <button
                      className="primary start-button"
                      onClick={create}
                      disabled={busy}
                    >
                      <Plus size={16} />
                      建立第一個對話
                    </button>
                  )}
                </div>
              )}
              <div className="messages">
                {chat.messages.map((message, i) => (
                  <article key={i} className={`message ${message.role}`}>
                    <div className="message-avatar">
                      {message.role === "user" ? (
                        "Y"
                      ) : message.role === "tool" ? (
                        <Terminal size={15} />
                      ) : (
                        <Hexagon size={17} />
                      )}
                    </div>
                    <div className="message-body">
                      <div className="message-author">
                        {message.role === "user"
                          ? "你"
                          : message.role === "tool"
                            ? "工具"
                            : "Hermes"}
                        <span>
                          {message.role === "assistant" ? "AGENT" : ""}
                        </span>
                      </div>
                      <div className="markdown">
                        <Markdown>{message.text}</Markdown>
                      </div>
                      {(message.role === "user" ||
                        message.role === "assistant") &&
                        message.text.trim() && (
                          <button
                            aria-label={`從第 ${i + 1} 則訊息分支`}
                            disabled={busy || chat.running || !status.connected}
                            onClick={() =>
                              run(async () => {
                                const count = chat.messages
                                  .slice(0, i + 1)
                                  .filter(
                                    (m) =>
                                      (m.role === "user" ||
                                        m.role === "assistant") &&
                                      m.text.trim(),
                                  ).length;
                                const data = await rpc("session.branch", {
                                  session_id: chat.session,
                                  count,
                                });
                                if (data.session_id)
                                  await hydrate(
                                    "session.activate",
                                    String(data.session_id),
                                  );
                                await refreshSessions();
                                setNotice("已建立訊息分支");
                              })
                            }
                          >
                            <GitBranch size={12} />
                            從這裡分支
                          </button>
                        )}
                    </div>
                  </article>
                ))}
                {chat.running && (
                  <div className="thinking">
                    <span />
                    <span />
                    <span /> Hermes 正在處理
                  </div>
                )}
                {chat.tools.length > 0 && (
                  <details className="tool-tray" open>
                    <summary>
                      <Terminal size={14} /> 這次執行的工具 ·{" "}
                      {chat.tools.length}
                    </summary>
                    {chat.tools.map((tool) => (
                      <div key={tool.id}>
                        <span className={tool.done ? "tool-done" : ""}>
                          {tool.done ? (
                            <Check size={13} />
                          ) : (
                            <Activity size={13} />
                          )}
                        </span>
                        <strong>{tool.name}</strong>
                        <pre>
                          {tool.detail || (tool.done ? "完成" : "執行中…")}
                        </pre>
                      </div>
                    ))}
                  </details>
                )}
                <div ref={end} />
              </div>
            </div>
            <div className="composer-area">
              <form className="composer" onSubmit={send}>
                <textarea
                  aria-label="訊息"
                  value={text}
                  placeholder={
                    status.connected
                      ? chat.session
                        ? "告訴 Hermes 你想做什麼…"
                        : "先新增對話，開始工作…"
                      : "連接 Hermes 後開始對話…"
                  }
                  onChange={(e) => setText(e.target.value)}
                  disabled={!status.connected || !chat.session}
                  onKeyDown={(e) => {
                    if (
                      e.key === "Enter" &&
                      !e.shiftKey &&
                      !e.nativeEvent.isComposing
                    ) {
                      e.preventDefault();
                      send();
                    }
                  }}
                />
                <div className="composer-bottom">
                  <span>
                    <Folder size={13} />
                    <span title={settings.workspace}>
                      {settings.workspace || "Hermes 預設工作目錄"}
                    </span>
                  </span>
                  <div>
                    {chat.running ? (
                      <button
                        type="button"
                        className="stop-button"
                        aria-label="中止執行"
                        onClick={() =>
                          run(async () => {
                            await rpc("session.interrupt", {
                              session_id: chat.session,
                            });
                            setNotice("已送出中止請求");
                          })
                        }
                      >
                        <Square size={13} />
                        中止
                      </button>
                    ) : (
                      <>
                        <span className="key-hint">Enter 傳送</span>
                        <button
                          className="send-button"
                          type="submit"
                          aria-label="傳送訊息"
                          disabled={
                            !text.trim() ||
                            !chat.session ||
                            !status.connected ||
                            busy
                          }
                        >
                          <ArrowUp size={18} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
              </form>
              <p className="composer-note">
                <ShieldCheck size={12} />{" "}
                工具操作可能影響工作目錄，執行前請確認核准內容。
              </p>
            </div>
          </>
        )}
        {page === "settings" && (
          <div className="page-content">
            <div className="page-heading">
              <span className="eyebrow">CONNECTION & PREFERENCES</span>
              <h1>讓工作台準備就緒。</h1>
              <p>連接伺服器上的 Hermes，設定你的工作環境。</p>
            </div>
            <div className="settings-layout">
              <SecuritySettings request={api} />
              <WorkspaceFiles request={api} />
              <MaintenanceSettings request={api} />
              <section className="panel">
                <div className="panel-heading">
                  <div className="panel-symbol">
                    <Terminal size={19} />
                  </div>
                  <div>
                    <h2>Hermes 執行環境</h2>
                    <p>使用已安裝 Hermes 依賴的 Python 環境。</p>
                  </div>
                  <span className="tag">LOCAL PROCESS</span>
                </div>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    run(async () => {
                      await api("/settings", "PUT", settings);
                      setNotice("連線設定已儲存；重新連線後生效");
                    });
                  }}
                >
                  <label>
                    Python 執行檔
                    <input
                      value={settings.python}
                      aria-label="Python 執行檔"
                      onChange={(e) =>
                        setSettings({ ...settings, python: e.target.value })
                      }
                      placeholder="/opt/hermes/.venv/bin/python"
                      required
                    />
                    <small>建議填入 Hermes 虛擬環境的絕對路徑。</small>
                  </label>
                  <label>
                    Hermes 原始碼目錄
                    <input
                      value={settings.source}
                      aria-label="Hermes 原始碼目錄"
                      onChange={(e) =>
                        setSettings({ ...settings, source: e.target.value })
                      }
                      placeholder="/opt/hermes-agent"
                      required
                    />
                  </label>
                  <label>
                    預設工作目錄
                    <input
                      value={settings.workspace}
                      aria-label="預設工作目錄"
                      onChange={(e) =>
                        setSettings({ ...settings, workspace: e.target.value })
                      }
                      placeholder="/srv/workspace"
                    />
                    <small>僅套用至新對話；留空沿用 Hermes 預設。</small>
                  </label>
                  <label>
                    Hermes Home（選填）
                    <input
                      value={settings.home}
                      aria-label="Hermes Home"
                      onChange={(e) =>
                        setSettings({ ...settings, home: e.target.value })
                      }
                      placeholder="留空使用既有 HERMES_HOME"
                    />
                    <small>
                      指定既有目錄，與 CLI 共用設定時請避免同時執行其他
                      gateway。
                    </small>
                  </label>
                  <div className="panel-footer">
                    <span>儲存後手動重新連線</span>
                    <button className="primary" disabled={busy}>
                      <Check size={15} />
                      儲存連線設定
                    </button>
                  </div>
                </form>
              </section>
              <aside className="connection-card">
                <span className="eyebrow">GATEWAY STATUS</span>
                <div
                  className={`connection-visual ${status.connected ? "connected" : ""}`}
                >
                  <Hexagon size={40} />
                  <span
                    className={`status-dot ${status.connected ? "online" : ""}`}
                  />
                </div>
                <h3>{stateNames[status.state] ?? status.state}</h3>
                <p>
                  {status.connected
                    ? "Hermes 已準備好接收你的工作。"
                    : "設定執行環境後，啟動 Hermes 連線。"}
                </p>
                {status.error && (
                  <pre className="connection-error">{status.error}</pre>
                )}
                <button
                  className="primary"
                  disabled={busy || !settings.source}
                  onClick={() =>
                    run(async () => {
                      setStatus(await api<Status>("/restart", "POST", {}));
                      await refreshSessions();
                      setNotice("Hermes 已連線");
                    })
                  }
                >
                  <RefreshCw size={15} />
                  {busy ? "處理中…" : "啟動／重新連線"}
                </button>
                <button
                  disabled={busy || !status.connected}
                  onClick={() =>
                    run(async () => {
                      setStatus(await api<Status>("/stop", "POST", {}));
                      setChat(initialChat(""));
                      setSessions([]);
                    })
                  }
                >
                  <Power size={14} />
                  停止 Hermes
                </button>
                <small>重新連線會中止目前執行中的工作。</small>
              </aside>
            </div>
            <section className="panel install-panel">
              <div className="panel-heading">
                <div className="panel-symbol">
                  <ArrowDown size={19} />
                </div>
                <div>
                  <h2>還沒有安裝 Hermes？</h2>
                  <p>從官方儲存庫下載原始碼、建立虛擛環境並安裝依賴。</p>
                </div>
                <span className="tag">ONE CLICK</span>
              </div>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  startInstall();
                }}
              >
                <label>
                  基底 Python
                  <input
                    value={install.python}
                    aria-label="基底 Python"
                    onChange={(e) =>
                      setInstall({ ...install, python: e.target.value })
                    }
                    placeholder="python3（用來建立虛擬環境的直譯器）"
                    required
                  />
                  <small>伺服器上已安裝、可上網的 Python 直譯器。</small>
                </label>
                <label>
                  安裝目錄
                  <input
                    value={install.dir}
                    aria-label="安裝目錄"
                    onChange={(e) =>
                      setInstall({ ...install, dir: e.target.value })
                    }
                    placeholder="/opt/hermes-agent"
                    required
                  />
                  <small>必須是新的或空的絕對路徑目錄。</small>
                </label>
                <label>
                  分支或標籤
                  <input
                    value={install.ref}
                    onChange={(e) =>
                      setInstall({ ...install, ref: e.target.value })
                    }
                    placeholder="main"
                  />
                  <small>
                    預設 main；模型供應商登入與 API key 仍依 Hermes
                    官方流程設定。
                  </small>
                </label>
                <div className="panel-footer">
                  <span>下載 → venv → pip → 驗證 tui_gateway</span>
                  <button
                    className="primary"
                    disabled={busy || installSt?.running}
                  >
                    <ArrowDown size={15} />
                    {installSt?.running ? "安裝中…" : "下載並安裝"}
                  </button>
                </div>
              </form>
              {installSt && (installSt.running || installSt.done) && (
                <div className="install-progress">
                  {installSt.steps.length > 0 && (
                    <ol className="install-steps">
                      {installSt.steps.map((s) => (
                        <li key={s}>{s}</li>
                      ))}
                    </ol>
                  )}
                  {installSt.error && (
                    <div className="error" role="alert">
                      {installSt.error}
                    </div>
                  )}
                  {installSt.log.length > 0 && (
                    <pre className="install-log">
                      {installSt.log.join("\n")}
                    </pre>
                  )}
                </div>
              )}
            </section>
            <section className="panel hermes-preferences">
              <div className="panel-heading">
                <div className="panel-symbol">
                  <Settings2 size={19} />
                </div>
                <div>
                  <h2>Hermes 偏好設定</h2>
                  <p>直接讀取與寫入 Hermes 設定。</p>
                </div>
              </div>
              <div className="config-fields">
                <label>
                  設定項目
                  <select
                    value={configKey}
                    onChange={(e) => {
                      setConfigKey(e.target.value);
                      setConfigValue("");
                    }}
                  >
                    <option value="prompt">自訂提示詞</option>
                    <option value="reasoning">推理程度</option>
                    <option value="personality">個性</option>
                  </select>
                </label>
                <label>
                  設定值
                  <textarea
                    aria-label="Hermes 設定值"
                    value={configValue}
                    onChange={(e) => setConfigValue(e.target.value)}
                    placeholder={
                      configKey === "prompt"
                        ? "輸入自訂提示詞；clear 可清除"
                        : configKey === "reasoning"
                          ? "例如 medium、high"
                          : "例如 none"
                    }
                  />
                </label>
              </div>
              <div className="panel-footer">
                <span>套用至 Hermes 設定與目前對話（若支援）</span>
                <div className="button-row">
                  <button
                    disabled={busy || !status.connected}
                    onClick={() =>
                      run(async () => {
                        const result = await rpc("config.get", {
                          key: configKey,
                        });
                        setConfigValue(
                          String(result.prompt ?? result.value ?? ""),
                        );
                      })
                    }
                  >
                    讀取設定
                  </button>
                  <button
                    className="primary"
                    disabled={busy || !status.connected || !configValue.trim()}
                    onClick={() =>
                      run(async () => {
                        await rpc("config.set", {
                          key: configKey,
                          value: configValue,
                          ...(chat.session ? { session_id: chat.session } : {}),
                        });
                        setNotice("Hermes 設定已更新");
                      })
                    }
                  >
                    套用設定
                  </button>
                </div>
              </div>
            </section>
          </div>
        )}
        {page === "logs" && (
          <div className="page-content">
            <div className="page-heading">
              <span className="eyebrow">ACTIVITY</span>
              <h1>每一步，都有跡可循。</h1>
              <p>控制台操作紀錄與目前瀏覽器收到的 Hermes 事件。</p>
            </div>
            {usage && (usage.totals?.total || usage.items?.length) ? (
              <div className="usage-card">
                <div>
                  <span>回合數</span>
                  <strong>{usage.totals.calls}</strong>
                </div>
                <div>
                  <span>輸入 tokens</span>
                  <strong>{Math.round(usage.totals.input)}</strong>
                </div>
                <div>
                  <span>輸出 tokens</span>
                  <strong>{Math.round(usage.totals.output)}</strong>
                </div>
                <div>
                  <span>總 tokens</span>
                  <strong>{Math.round(usage.totals.total)}</strong>
                </div>
              </div>
            ) : null}
            <div className="log-heading">
              <h2>
                活動紀錄 <span>{logs.length + events.length}</span>
              </h2>
              <button
                disabled={busy}
                onClick={() =>
                  run(async () => setLogs(await api<Entry[]>("/logs")))
                }
              >
                <RefreshCw size={14} />
                重新整理
              </button>
            </div>
            <div className="log-list">
              {[...events, ...logs]
                .sort((a, b) => b.time.localeCompare(a.time))
                .map((entry, i) => (
                  <div
                    className="log-row"
                    key={`${entry.kind}-${entry.id}-${i}`}
                  >
                    <div className="log-icon">
                      {entry.kind === "event" ? (
                        <Activity size={15} />
                      ) : (
                        <Terminal size={15} />
                      )}
                    </div>
                    <span className="log-kind">{entry.kind.toUpperCase()}</span>
                    <strong>{entry.detail}</strong>
                    <time>
                      {new Date(entry.time).toLocaleString("zh-TW", {
                        hour12: false,
                      })}
                    </time>
                  </div>
                ))}
              {!logs.length && !events.length && (
                <div className="empty-log">
                  <Activity size={28} />
                  <h3>一切從這裡開始</h3>
                  <p>連接 Hermes 後，操作紀錄會顯示於此。</p>
                </div>
              )}
            </div>
            <details className="diag-box">
              <summary>Hermes 診斷（stderr 尾端）</summary>
              <pre>{diag || "目前沒有 Hermes 診斷輸出。"}</pre>
            </details>
            <p className="muted log-foot">
              控制台保留最近 1,000 筆操作，本頁顯示最近 200
              筆；串流事件僅保留於目前瀏覽器。用量資訊由 Hermes 回報後累計。
            </p>
          </div>
        )}
        {chat.requests.length > 0 && (
          <div className="request-stack">
            {chat.requests.map((frame) => (
              <RequestCard
                key={frame.id}
                frame={frame}
                busy={busy}
                onRespond={(result) => run(() => respond(frame, result))}
              />
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
function RequestCard({
  frame,
  busy,
  onRespond,
}: {
  frame: Frame;
  busy: boolean;
  onRespond: (result: unknown) => void;
}) {
  const [answer, setAnswer] = useState("{}"),
    p = frame.params ?? {};
  return (
    <section className="request-card">
      <div className="request-title">
        <ShieldCheck size={18} />
        <h3>
          {frame.method === "approval" ? "需要你的核准" : "Hermes 需要更多資訊"}
        </h3>
      </div>
      <small>對話 {p.session_id ?? "目前對話"}</small>
      <p>{String(p.description ?? p.question ?? "請確認以下內容")}</p>
      {frame.method === "approval" ? (
        <>
          <pre>{String(p.command ?? "工具操作")}</pre>
          <div className="button-row">
            <button
              disabled={busy}
              onClick={() => onRespond({ choice: "deny" })}
            >
              拒絕
            </button>
            <button
              className="primary"
              disabled={busy}
              onClick={() => onRespond({ choice: "once" })}
            >
              允許這一次
            </button>
          </div>
        </>
      ) : (
        <>
          <pre>{JSON.stringify(p.questions ?? p, null, 2)}</pre>
          <label>
            回答（問題 ID 對應答案的 JSON）
            <textarea
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
            />
          </label>
          <div className="button-row">
            <button disabled={busy} onClick={() => onRespond({})}>
              取消回答
            </button>
            <button
              className="primary"
              disabled={busy}
              onClick={() => {
                try {
                  const value = JSON.parse(answer);
                  if (
                    !value ||
                    Array.isArray(value) ||
                    typeof value !== "object"
                  )
                    throw Error();
                  onRespond({ answers: value });
                } catch {
                  setAnswer('請輸入有效的 JSON 物件，例如 {"q1":"答案"}');
                }
              }}
            >
              提交回答
            </button>
          </div>
        </>
      )}
    </section>
  );
}
