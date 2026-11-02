export type Message = { role: string; text: string };
export type Frame = {
  id?: string;
  method: string;
  params?: Record<string, any>;
};
export type Chat = {
  session: string;
  messages: Message[];
  running: boolean;
  requests: Frame[];
  tools: { id: string; name: string; done: boolean; detail: string }[];
};
export function initialChat(session: string): Chat {
  return { session, messages: [], running: false, requests: [], tools: [] };
}
export function applyFrame(state: Chat, frame: Frame): Chat {
  const p = frame.params ?? {};
  if (frame.method === "console.request.resolved")
    return { ...state, requests: state.requests.filter((r) => r.id !== p.id) };
  if (frame.method === "approval" || frame.method === "clarify")
    return {
      ...state,
      requests: [...state.requests.filter((r) => r.id !== frame.id), frame],
    };
  if (frame.method !== "event") return state;
  const type = p.type as string,
    payload = p.payload ?? {};
  if (type === "request.cancel")
    return {
      ...state,
      requests: state.requests.filter((r) => r.id !== payload.id),
    };
  if (p.session_id !== state.session) return state;
  if (type === "message.start") return { ...state, running: true };
  if (type === "message.delta" || type === "message.complete") {
    const messages = [...state.messages],
      last = messages.at(-1),
      text = typeof payload.text === "string" ? payload.text : "";
    if (last?.role === "assistant" && state.running)
      messages[messages.length - 1] = {
        ...last,
        text: type === "message.delta" ? last.text + text : text || last.text,
      };
    else if (text) messages.push({ role: "assistant", text });
    return { ...state, messages, running: type !== "message.complete" };
  }
  if (
    type === "tool.start" ||
    type === "tool.complete" ||
    type === "tool.generating"
  ) {
    const id = String(payload.tool_id ?? payload.id ?? payload.name ?? "tool"),
      old = state.tools.find((t) => t.id === id);
    const tool = {
      id,
      name: String(payload.name ?? payload.tool ?? old?.name ?? "tool"),
      done: type === "tool.complete",
      detail: String(
        payload.preview ??
          payload.output ??
          payload.result ??
          payload.description ??
          "",
      ),
    };
    return {
      ...state,
      tools: [...state.tools.filter((t) => t.id !== id), tool].slice(-30),
    };
  }
  if (
    type === "error" ||
    type === "session.error" ||
    type === "message.interrupted"
  )
    return { ...state, running: false };
  return state;
}
export function historyMessages(rows: Record<string, any>[]): Message[] {
  return rows
    .filter(
      (r) =>
        r.display_kind !== "hidden" &&
        ["user", "assistant", "tool"].includes(r.role),
    )
    .map((r) => ({
      role: r.role,
      text: String(
        r.display_metadata?.display_text ??
          r.text ??
          r.content ??
          (r.name ? `工具：${r.name}` : ""),
      ),
    }))
    .filter((r) => r.text.length > 0);
}
