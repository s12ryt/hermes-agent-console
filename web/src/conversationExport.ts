import type { Message } from "./state";
export function conversationExport(
  session: string,
  messages: Message[],
  format: "json" | "md",
): string {
  if (format === "json") return JSON.stringify({ session, messages }, null, 2);
  return (
    "# Hermes 對話\n\n" +
    messages.map((m) => `## ${m.role}\n\n${m.text}`).join("\n\n---\n\n") +
    "\n"
  );
}
export function downloadConversation(
  session: string,
  messages: Message[],
  format: "json" | "md",
) {
  const blob = new Blob([conversationExport(session, messages, format)], {
    type:
      format === "json" ? "application/json" : "text/markdown;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `hermes-conversation.${format}`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
