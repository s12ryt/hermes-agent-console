import { describe, expect, it } from "vitest";
import { applyFrame, initialChat, historyMessages } from "./state";
describe("Hermes event projection", () => {
  it("isolates sessions and appends deltas without duplicating completion", () => {
    let state = initialChat("s1");
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "message.delta",
        session_id: "other",
        payload: { text: "wrong" },
      },
    });
    expect(state.messages).toHaveLength(0);
    state = applyFrame(state, {
      method: "event",
      params: { type: "message.start", session_id: "s1", payload: {} },
    });
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "message.delta",
        session_id: "s1",
        payload: { text: "Hello " },
      },
    });
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "message.delta",
        session_id: "s1",
        payload: { text: "world" },
      },
    });
    expect(state.messages[0].text).toBe("Hello world");
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "message.complete",
        session_id: "s1",
        payload: { text: "Hello world!" },
      },
    });
    expect(state.messages).toHaveLength(1);
    expect(state.messages[0].text).toBe("Hello world!");
    expect(state.running).toBe(false);
  });
  it("records tools and resolves canceled interaction requests", () => {
    let state = initialChat("s1");
    state = applyFrame(state, {
      id: "r1",
      method: "approval",
      params: { command: "ls", session_id: "s1" },
    });
    expect(state.requests[0].id).toBe("r1");
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "request.cancel",
        session_id: "s1",
        payload: { id: "r1" },
      },
    });
    expect(state.requests).toHaveLength(0);
    state = applyFrame(state, {
      method: "event",
      params: {
        type: "tool.start",
        session_id: "s1",
        payload: { name: "terminal", tool_id: "t1" },
      },
    });
    expect(state.tools[0].name).toBe("terminal");
  });
  it("uses display text and removes hidden scaffolding", () => {
    expect(
      historyMessages([
        { role: "user", text: "hidden", display_kind: "hidden" },
        {
          role: "assistant",
          text: "raw",
          display_kind: "process_complete",
          display_metadata: { display_text: "Readable" },
        },
      ]),
    ).toEqual([{ role: "assistant", text: "Readable" }]);
  });
});
