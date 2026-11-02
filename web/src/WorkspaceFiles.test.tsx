import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import WorkspaceFiles from "./WorkspaceFiles";

it("browses directories, previews text safely and returns to the parent", async () => {
  const request = vi.fn(async (path: string) => {
    if (path === "/files?path=.")
      return { path: ".", entries: [{ name: "src", dir: true }] };
    if (path === "/files?path=src")
      return { path: "src", entries: [{ name: "hello.txt", dir: false }] };
    return { path: "src/hello.txt", content: "<script>alert('x')</script>" };
  });
  const user = userEvent.setup();
  render(<WorkspaceFiles request={request} />);
  await user.click(screen.getByRole("button", { name: "瀏覽工作目錄" }));
  await user.click(await screen.findByRole("button", { name: "開啟 src" }));
  await user.click(
    await screen.findByRole("button", { name: "開啟 hello.txt" }),
  );
  expect(
    await screen.findByText("<script>alert('x')</script>"),
  ).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
  await user.click(screen.getByRole("button", { name: "上一層" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith("/files?path=src"),
  );
});

it("shows a failed read and allows retry", async () => {
  const request = vi
    .fn()
    .mockRejectedValueOnce(new Error("請先設定工作目錄"))
    .mockResolvedValue({ path: ".", entries: [], truncated: true });
  const user = userEvent.setup();
  render(<WorkspaceFiles request={request} />);
  await user.click(screen.getByRole("button", { name: "瀏覽工作目錄" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "請先設定工作目錄",
  );
  await user.click(screen.getByRole("button", { name: "瀏覽工作目錄" }));
  expect(await screen.findByText("只顯示前 500 筆項目")).toBeInTheDocument();
});
