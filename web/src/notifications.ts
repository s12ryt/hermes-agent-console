import type { Frame } from "./state";
export function notifyFrame(frame: Frame) {
  if (
    typeof Notification === "undefined" ||
    Notification.permission !== "granted" ||
    document.visibilityState !== "hidden"
  )
    return;
  const kind = String(frame.params?.type ?? frame.method);
  const title =
    kind === "message.complete"
      ? "Hermes 回覆完成"
      : frame.method === "approval"
        ? "Hermes 等待核准"
        : frame.method === "clarify"
          ? "Hermes 等待回答"
          : "";
  if (!title) return;
  try {
    new Notification(title, {
      body: "請返回面板查看。",
      tag: "hermes-" + kind,
    });
  } catch {
    // Some browsers require service-worker notifications despite reporting permission.
    // Desktop hints are optional; the incoming chat frame must still be processed.
  }
}
