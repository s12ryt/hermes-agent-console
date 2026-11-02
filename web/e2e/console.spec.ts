import { expect, test } from "@playwright/test";

// 真實使用流程 E2E：以真實建置的 Go 執行檔 + 真實 Python 子程序（協定 fixture）
// 走完整使用者路徑。fixture 不是 Hermes、不呼叫模型、不執行系統命令。

const password = process.env.E2E_PASSWORD ?? "e2e-test-password-2026";
const python = process.env.E2E_PYTHON ?? "/usr/bin/python3";
const fixtureDir = process.env.E2E_FIXTURE_DIR ?? "";

test("login → connect → chat → approve → interrupt → logout", async ({
  page,
}) => {
  test.skip(!fixtureDir, "E2E_FIXTURE_DIR is required");

  // 未登入顯示登入頁
  await page.goto("/");
  await expect(page.getByText("進入你的工作台")).toBeVisible();

  // 錯誤密碼被拒
  await page.locator('input[type="password"]').fill("wrong-password");
  await page.getByRole("button", { name: /登入控制台/ }).click();
  await expect(page.locator('[role="alert"]')).toBeVisible();

  // 正確密碼登入
  await page.locator('input[type="password"]').fill(password);
  await page.getByRole("button", { name: /登入控制台/ }).click();
  await expect(page.getByRole("button", { name: /連線設定/ })).toBeVisible();

  // 填入執行環境並儲存
  await page.getByRole("button", { name: /連線設定/ }).first().click();
  await page.getByLabel("Python 執行檔").fill(python);
  await page.getByLabel("Hermes 原始碼目錄").fill(fixtureDir);
  await page.getByRole("button", { name: /儲存連線設定/ }).click();
  await expect(page.getByText(/連線設定已儲存/)).toBeVisible();

  // 啟動連線，等到「新增對話」可用（gateway.ready + capabilities 完成）
  await page.getByRole("button", { name: /啟動／重新連線/ }).click();
  await expect(page.getByRole("button", { name: /新增對話/ })).toBeEnabled({
    timeout: 30_000,
  });

  // 新對話 + 送出訊息 → 串流回覆 → 核准卡
  await page.getByRole("button", { name: /新增對話/ }).click();
  const box = page.getByRole("textbox", { name: "訊息" });
  await expect(box).toBeVisible();
  await box.fill("E2E：驗證串流與核准");
  await page.getByLabel("傳送訊息").click();
  await expect(page.getByText("正在驗證串流與工具核准…")).toBeVisible({
    timeout: 20_000,
  });
  await expect(
    page.getByRole("heading", { name: "需要你的核准" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "允許這一次" }).click();
  await expect(page.getByText(/已收到核准回應/)).toBeVisible({
    timeout: 20_000,
  });

  // 第二輪：等待核准時中止
  await box.fill("E2E：驗證中止");
  await page.getByLabel("傳送訊息").click();
  await expect(
    page.getByRole("heading", { name: "需要你的核准" }),
  ).toBeVisible();
  await page.getByLabel("中止執行").click();
  await expect(page.getByText("Fixture 已中止。")).toBeVisible({
    timeout: 20_000,
  });

  // 登出回到登入頁
  await page.getByLabel("登出").click();
  await expect(page.getByText("進入你的工作台")).toBeVisible();
});
