import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SecuritySettings from "./SecuritySettings";

it("saves a provider key, clears the secret and manages exact approval rules", async () => {
  const calls: { path: string; body?: unknown }[] = [];
  const request = vi.fn(
    async (path: string, _method?: string, body?: unknown): Promise<any> => {
      calls.push({ path, body });
      if (path === "/keys") return body ? {} : [{ id: 1, name: "openrouter" }];
      if (path === "/rules") return body ? {} : [{ id: 2, pattern: "ls" }];
      return {};
    },
  );
  const user = userEvent.setup();
  render(<SecuritySettings request={request} />);
  await user.type(
    await screen.findByLabelText("供應商 API Key"),
    "sk-test-secret",
  );
  await user.click(screen.getByRole("button", { name: "儲存金鑰" }));
  await waitFor(() =>
    expect(calls).toContainEqual({
      path: "/keys",
      body: { name: "openrouter", value: "sk-test-secret" },
    }),
  );
  expect(screen.getByLabelText("供應商 API Key")).toHaveValue("");
  await user.type(screen.getByLabelText("完整核准命令"), "ls -la");
  await user.click(screen.getByRole("button", { name: "新增核准規則" }));
  await waitFor(() =>
    expect(calls).toContainEqual({
      path: "/rules",
      body: { pattern: "ls -la" },
    }),
  );
  await user.click(
    screen.getByRole("button", { name: "刪除 openrouter 金鑰" }),
  );
  await user.click(screen.getByRole("button", { name: "刪除規則 ls" }));
  expect(calls).toContainEqual({ path: "/keys/delete", body: { id: 1 } });
  expect(calls).toContainEqual({ path: "/rules/delete", body: { id: 2 } });
});
