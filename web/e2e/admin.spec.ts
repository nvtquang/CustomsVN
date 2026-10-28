import { expect, test } from "@playwright/test";

test("dashboard shows the day's questions and costs", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: "Tổng quan" })).toBeVisible();
  await expect(page.getByText("Câu hỏi mỗi ngày")).toBeVisible();
  await page.getByLabel("Xem dạng bảng").check();
  await expect(page.getByRole("columnheader", { name: "Ngày" })).toBeVisible();
});

test("data: versions, coverage and a job with its log", async ({ page }) => {
  await page.goto("/admin/data");
  await expect(page.getByText("Các phiên bản corpus")).toBeVisible();
  await expect(page.getByText("đang dùng")).toBeVisible();
  // A real ingest rewrites the relation tables for minutes and blocks the
  // answers meanwhile, so the test queues one and cancels it.
  await page.getByLabel("Loại job").selectOption("ingest");
  await page.getByRole("button", { name: "Chạy", exact: true }).click();
  await page.getByRole("button", { name: "Hủy", exact: true }).click();
  await expect(page.getByLabel("Log của job")).toContainText("canceled", { timeout: 60_000 });
});

test("users: create one with a generated password", async ({ page }) => {
  await page.goto("/admin/users");
  const email = `e2e-${Date.now()}@example.com`;
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Tạo tài khoản" }).click();
  await expect(page.getByText(`Đã tạo ${email}`)).toBeVisible();
  const row = page.getByRole("row", { name: new RegExp(email) });
  await expect(row).toBeVisible();
  page.once("dialog", (d) => void d.accept());
  await row.getByRole("button", { name: "Xóa" }).click();
  await expect(row).toHaveCount(0);
});

test("config export holds no key", async ({ page }) => {
  await page.goto("/settings/config");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Tải tệp JSON" }).click();
  const file = await (await download).path();
  const text = await (await import("node:fs/promises")).readFile(file, "utf8");
  const cfg = JSON.parse(text);
  expect(cfg.format).toBe("customsvn-config");
  expect(text).not.toMatch(/api_key"\s*:/);
  expect(cfg.llm.providers[0].name).toBe("mock");
});

test("answer evaluation runs a gold set and shows its metrics", async ({ page }) => {
  await page.goto("/lab/answers");
  const set = page.getByLabel("Chọn gold set");
  const option = set.locator("option", { hasText: /^e2e v/ }).first();
  await set.selectOption({ label: (await option.textContent())! });
  await page.getByLabel("Nhãn lần chạy").fill("e2e mock");
  await page.getByRole("button", { name: "Chạy", exact: true }).click();
  await expect(page.getByText("Từ chối đúng").first()).toBeVisible();
  // Newest run first; earlier runs of the suite carry the same label.
  await expect(
    page
      .getByRole("row", { name: /e2e mock/ })
      .first()
      .getByText("Xong", { exact: true }),
  ).toBeVisible({
    timeout: 120_000,
  });
});
