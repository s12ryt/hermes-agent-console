import { afterEach, expect, it, vi } from "vitest";
import { notifyFrame } from "./notifications";
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
it("only notifies permitted background tabs and never includes message content", () => {
  const notification = vi.fn(function () {});
  Object.assign(notification, { permission: "granted" });
  vi.stubGlobal("Notification", notification);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  notifyFrame({
    method: "event",
    params: { type: "message.complete", payload: { text: "secret" } },
  });
  expect(notification).toHaveBeenCalledWith("Hermes 回覆完成", {
    body: "請返回面板查看。",
    tag: "hermes-message.complete",
  });
  notification.mockClear();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  notifyFrame({ method: "approval" });
  expect(notification).not.toHaveBeenCalled();
});
