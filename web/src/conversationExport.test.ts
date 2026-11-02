import { expect, it } from "vitest";
import { conversationExport } from "./conversationExport";
it("exports visible conversation messages as JSON and Markdown", () => {
  const messages = [
    { role: "user", text: "問題" },
    { role: "assistant", text: "答案\n```js\n42\n```" },
  ];
  expect(JSON.parse(conversationExport("s1", messages, "json"))).toEqual({
    session: "s1",
    messages,
  });
  expect(conversationExport("s1", messages, "md")).toContain(
    "## assistant\n\n答案\n```js\n42\n```",
  );
});
