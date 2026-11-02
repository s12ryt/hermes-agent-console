import { expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";

const calls: { url: string; body: any }[] = [];
it("refreshes the local search index after a completed turn", async () => {
  signedIn = true;
  render(<App />);
  await screen.findByRole("button", { name: "啟用桌面通知" });
  await waitFor(() => expect(Events.instances.length).toBeGreaterThan(0));
  Events.instances
    .at(-1)!
    .emit("frame", {
      method: "event",
      params: { type: "message.complete", session_id: "completed-1" },
    });
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.history" &&
          c.body.params.session_id === "completed-1",
      ),
    ).toBe(true),
  );
});
let signedIn = false;
it("archives stored sessions and confirms permanent deletion", async () => {
  signedIn = true;
  vi.spyOn(window, "confirm").mockReturnValue(true);
  const user = userEvent.setup();
  render(<App />);
  await user.click(
    await screen.findByRole("button", { name: "歸檔 既有對話" }),
  );
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.archive" &&
          c.body.params.session_id === "stored-1" &&
          c.body.params.archived === true,
      ),
    ).toBe(true),
  );
  await user.click(screen.getByRole("button", { name: "刪除 既有對話" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.delete" &&
          c.body.params.session_id === "stored-1",
      ),
    ).toBe(true),
  );
});
let installPolls = 0;
it("branches at the selected visible message", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await user.click(await screen.findByRole("button", { name: "既有對話" }));
  await user.click(
    await screen.findByRole("button", { name: "從第 1 則訊息分支" }),
  );
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.branch" &&
          c.body.params.count === 1 &&
          c.body.params.session_id === "resumed-1",
      ),
    ).toBe(true),
  );
});
class Events {
  static instances: Events[] = [];
  listeners: Record<string, (e: { data: string }) => void> = {};
  constructor() {
    Events.instances.push(this);
  }
  addEventListener(name: string, fn: any) {
    this.listeners[name] = fn;
  }
  close() {}
  emit(name: string, data: any) {
    this.listeners[name]?.({ data: JSON.stringify(data) });
  }
}
beforeEach(() => {
  signedIn = false;
  installPolls = 0;
  calls.length = 0;
  Events.instances = [];
  vi.stubGlobal("EventSource", Events);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const body = init?.body ? JSON.parse(String(init.body)) : {};
      calls.push({ url, body });
      let status = 200,
        data: any = {};
      if (url === "/api/me" && !signedIn) {
        status = 401;
        data = { error: "請先登入" };
      } else if (url === "/api/login") {
        if (body.password === "wrong") {
          status = 401;
          data = { error: "密碼不正確" };
        } else signedIn = true;
      } else if (url === "/api/status")
        data = { connected: true, state: "ready" };
      else if (url === "/api/settings")
        data = {
          python: "python",
          source: "/opt/hermes",
          workspace: "/srv/work",
          home: "",
        };
      else if (url === "/api/logs") data = [];
      else if (url === "/api/usage")
        data = {
          items: [
            {
              time: "2026-11-02T00:00:00Z",
              session_id: "s1",
              calls: 1,
              input: 120,
              output: 80,
              total: 200,
            },
          ],
          totals: { calls: 3, input: 130, output: 85, total: 215 },
        };
      else if (url === "/api/diag") data = { stderr: "fixture stderr line" };
      else if (url === "/api/install/start") data = { ok: true };
      else if (url === "/api/install/status") {
        installPolls++;
        data =
          installPolls === 1
            ? {
                running: true,
                done: false,
                steps: ["下載 Hermes 原始碼（main）"],
                log: ["下載中…"],
              }
            : {
                running: false,
                done: true,
                steps: ["下載 Hermes 原始碼（main）", "驗證 tui_gateway 模組"],
                log: [],
                python: "/opt/hermes-new/.venv/bin/python",
                source: "/opt/hermes-new",
              };
      } else if (url === "/api/rpc") {
        if (body.method === "session.list")
          data = { sessions: [{ id: "stored-1", title: "既有對話" }] };
        if (body.method === "session.create")
          data = {
            session_id: "live-1",
            messages: [],
            info: { model: "test-model", cwd: "/srv/work" },
          };
        if (body.method === "session.resume")
          data = {
            session_id: "resumed-1",
            messages: [{ role: "assistant", text: "歷史訊息" }],
          };
        if (body.method === "session.history") data = { messages: [] };
        if (body.method === "session.title") data = { ok: true };
        if (body.method === "session.close")
          data = { closed: true, messages: [] };
        if (body.method === "session.compress") data = { ok: true };
        if (body.method === "session.branch_whole")
          data = { session_id: "branch-1", messages: [] };
        if (body.method === "session.activate")
          data = { session_id: "branch-1", messages: [] };
      }
      return { ok: status < 400, status, json: async () => data };
    }),
  );
});
it("rejects wrong login, then creates and sends a prompt and displays streamed reply", async () => {
  const user = userEvent.setup();
  render(<App />);
  await screen.findByLabelText("管理密碼");
  await user.type(screen.getByLabelText("管理密碼"), "wrong");
  await user.click(screen.getByRole("button", { name: "登入控制台" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("密碼不正確");
  await user.clear(screen.getByLabelText("管理密碼"));
  await user.type(screen.getByLabelText("管理密碼"), "correct");
  await user.click(screen.getByRole("button", { name: "登入控制台" }));
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "新增對話" }));
  await user.type(screen.getByLabelText("訊息"), "檢查這個專案");
  await user.click(screen.getByRole("button", { name: "傳送訊息" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "prompt.submit" &&
          c.body.params.text === "檢查這個專案" &&
          c.body.params.session_id === "live-1",
      ),
    ).toBe(true),
  );
  Events.instances.at(-1)!.emit("frame", {
    method: "event",
    params: {
      type: "message.delta",
      session_id: "live-1",
      payload: { text: "串流回覆" },
    },
  });
  expect(await screen.findByText("串流回覆")).toBeInTheDocument();
});
it("resumes stored sessions and saves connection settings", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await user.click(await screen.findByText("既有對話"));
  expect(await screen.findByText("歷史訊息")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "連線設定" }));
  const source = await screen.findByLabelText("Hermes 原始碼目錄");
  await user.clear(source);
  await user.type(source, "/new/hermes");
  await user.click(screen.getByRole("button", { name: "儲存連線設定" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) => c.url === "/api/settings" && c.body.source === "/new/hermes",
      ),
    ).toBe(true),
  );
});
it("shows approval and sends only explicit one-time choice", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await waitFor(() => expect(Events.instances.length).toBe(1));
  Events.instances[0].emit("frame", {
    id: "srq-1",
    method: "approval",
    params: {
      session_id: "s1",
      command: "rm file",
      description: "工具需要核准",
    },
  });
  await user.click(await screen.findByRole("button", { name: "允許這一次" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.url === "/api/respond" &&
          c.body.id === "srq-1" &&
          c.body.result.choice === "once",
      ),
    ).toBe(true),
  );
});
it("keeps the suggested prompt after creating the first session", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: /理解一個專案/ }));
  await waitFor(() =>
    expect(screen.getByLabelText("訊息")).toHaveValue(
      "請先閱讀目前工作目錄，說明專案結構與執行方式。",
    ),
  );
  expect(screen.getByLabelText("訊息")).toBeEnabled();
});
it("does not duplicate a rejected prompt when the user retries", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "新增對話" }));
  const original = globalThis.fetch;
  let rejected = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (...args: Parameters<typeof fetch>) => {
      if (
        String(args[0]) === "/api/rpc" &&
        String(args[1]?.body).includes("prompt.submit") &&
        !rejected
      ) {
        rejected = true;
        return {
          ok: false,
          status: 502,
          json: async () => ({ error: "Hermes 拒絕請求" }),
        } as Response;
      }
      return original(...args);
    }),
  );
  await user.type(screen.getByLabelText("訊息"), "重試訊息");
  await user.click(screen.getByRole("button", { name: "傳送訊息" }));
  await screen.findByRole("alert");
  await user.click(screen.getByRole("button", { name: "傳送訊息" }));
  await waitFor(() => expect(screen.getAllByText("重試訊息")).toHaveLength(1));
});
it("restores the stop control for a running resumed session", async () => {
  signedIn = true;
  const original = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (...args: Parameters<typeof fetch>) => {
      if (
        String(args[0]) === "/api/rpc" &&
        String(args[1]?.body).includes("session.resume")
      )
        return {
          ok: true,
          status: 200,
          json: async () => ({
            session_id: "running-session",
            messages: [],
            running: true,
          }),
        } as Response;
      return original(...args);
    }),
  );
  const user = userEvent.setup();
  render(<App />);
  await user.click(await screen.findByText("既有對話"));
  expect(
    await screen.findByRole("button", { name: "中止執行" }),
  ).toBeInTheDocument();
});

it("installs Hermes from the settings page and fills connection settings", async () => {
  const user = userEvent.setup();
  render(<App />);
  await user.type(await screen.findByLabelText("管理密碼"), "correct");
  await user.click(screen.getByRole("button", { name: "登入控制台" }));
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "連線設定" }));
  await user.type(screen.getByLabelText("基底 Python"), "python3");
  await user.type(screen.getByLabelText("安裝目錄"), "/opt/hermes-new");
  await user.click(screen.getByRole("button", { name: "下載並安裝" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.url === "/api/install/start" &&
          c.body.targetDir === "/opt/hermes-new",
      ),
    ).toBe(true),
  );
  expect(
    await screen.findByText("下載 Hermes 原始碼（main）"),
  ).toBeInTheDocument();
  await waitFor(
    () =>
      expect(screen.getByLabelText("Python 執行檔")).toHaveValue(
        "/opt/hermes-new/.venv/bin/python",
      ),
    { timeout: 5000 },
  );
}, 20000);

it("renames a stored session from the sidebar", async () => {
  signedIn = true;
  const prompt = vi.spyOn(window, "prompt").mockReturnValue("新名稱");
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "重新命名 既有對話" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.title" &&
          c.body.params.session_id === "stored-1" &&
          c.body.params.title === "新名稱",
      ),
    ).toBe(true),
  );
  prompt.mockRestore();
});

it("closes a stored session from the sidebar", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "關閉對話 既有對話" }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.close" &&
          c.body.params.session_id === "stored-1",
      ),
    ).toBe(true),
  );
});

it("compresses and branches the active session", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: "新增對話" }));
  await waitFor(() =>
    expect(calls.some((c) => c.body.method === "session.create")).toBe(true),
  );
  const compress = screen.getByRole("button", { name: "壓縮對話" });
  await waitFor(() => expect(compress).toBeEnabled());
  await user.click(compress);
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.compress" &&
          c.body.params.session_id === "live-1",
      ),
    ).toBe(true),
  );
  const branch = screen.getByRole("button", { name: "建立分支" });
  await waitFor(() => expect(branch).toBeEnabled());
  await user.click(branch);
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.body.method === "session.branch_whole" &&
          ["live-1", "resumed-1"].includes(c.body.params.session_id),
      ),
    ).toBe(true),
  );
});

it("shows the usage dashboard and diagnostics on the logs page", async () => {
  signedIn = true;
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText("既有對話");
  await user.click(screen.getByRole("button", { name: /執行紀錄/ }));
  expect(await screen.findByText("總 tokens")).toBeInTheDocument();
  expect(screen.getByText("215")).toBeInTheDocument();
  expect(await screen.findByText("fixture stderr line")).toBeInTheDocument();
});
