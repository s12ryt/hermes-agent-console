import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import MaintenanceSettings from "./MaintenanceSettings";
it("checks the published version on demand", async () => {
  const request = vi.fn(async () => ({
    current: "0.1.0",
    latest: "v0.2.0",
    url: "https://github.com/s12ryt/hermes-agent-console/releases/tag/v0.2.0",
  }));
  const user = userEvent.setup();
  render(<MaintenanceSettings request={request} />);
  await user.click(screen.getByRole("button", { name: "檢查面板更新" }));
  expect(
    await screen.findByRole("link", { name: "查看 v0.2.0" }),
  ).toHaveAttribute(
    "href",
    "https://github.com/s12ryt/hermes-agent-console/releases/tag/v0.2.0",
  );
  expect(request).toHaveBeenCalledWith("/updates");
});
it("validates backup JSON before restoring and displays the result", async () => {
  const request = vi.fn(async () => ({ ok: true }));
  const user = userEvent.setup();
  render(<MaintenanceSettings request={request} />);
  await user.upload(
    screen.getByLabelText("選擇設定備份"),
    new File(
      ['{"version":1,"settings":{},"approval_rules":[],"encrypted_keys":[]}'],
      "backup.json",
      { type: "application/json" },
    ),
  );
  await user.click(
    screen.getByRole("checkbox", {
      name: "我了解還原會覆寫目前設定、規則與金鑰",
    }),
  );
  await user.click(screen.getByRole("button", { name: "還原設定備份" }));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/restore",
      "POST",
      expect.objectContaining({ version: 1 }),
    ),
  );
  expect(await screen.findByRole("status")).toHaveTextContent("還原完成");
});
it("rejects malformed JSON without sending a restore", async () => {
  const request = vi.fn();
  const user = userEvent.setup();
  render(<MaintenanceSettings request={request} />);
  await user.upload(
    screen.getByLabelText("選擇設定備份"),
    new File(["broken"], "backup.json", { type: "application/json" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("有效的 JSON");
  expect(request).not.toHaveBeenCalled();
});
