import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ConversationSearch from "./ConversationSearch";
it("searches encoded content and opens the matching session", async () => {
  const request = vi.fn(async () => [
      { session_id: "s1", text: "資料庫 & 備份" },
    ]),
    open = vi.fn(async () => {}),
    user = userEvent.setup();
  render(<ConversationSearch request={request} open={open} />);
  await user.type(screen.getByLabelText("搜尋對話內容"), "資料庫 &");
  await user.click(screen.getByRole("button", { name: "搜尋內容" }));
  await user.click(await screen.findByRole("button", { name: /s1/ }));
  expect(request).toHaveBeenCalledWith(
    "/search?q=" + encodeURIComponent("資料庫 &"),
  );
  expect(open).toHaveBeenCalledWith("s1");
});
