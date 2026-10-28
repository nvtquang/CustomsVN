import { expect, test } from "@playwright/test";

test("the user menu leads to the settings, and closing them goes back", async ({ page }) => {
  await page.goto("/hs");
  await page.getByRole("button", { name: "Tài khoản và cài đặt" }).click();
  await expect(page.getByRole("menuitem", { name: "Quản trị" })).toBeVisible();
  await page.getByRole("menuitem", { name: "Cài đặt", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/general/);
  await expect(page.getByRole("heading", { name: "Cài đặt chung" })).toBeVisible();
  const nav = page.getByRole("navigation", { name: "Mục cài đặt" }).first();
  for (const name of ["Quản lý model", "Kiến thức và truy xuất", "Tổng quan", "Người dùng", "Kết nối MCP và API"]) {
    await expect(nav.getByRole("link", { name })).toBeVisible();
  }
  await page.getByRole("button", { name: "Đóng cài đặt" }).click();
  await expect(page).toHaveURL(/\/hs$/);
});

test("old settings addresses lead to the new pages", async ({ page }) => {
  await page.goto("/settings/llm");
  await expect(page).toHaveURL(/\/settings\/models/);
  await page.goto("/settings/retrieval");
  await expect(page).toHaveURL(/\/settings\/knowledge/);
});

test("models: add an embedding model after testing its connection, then remove it", async ({ page }) => {
  await page.goto("/settings/models");
  await expect(page.getByRole("heading", { name: "Quản lý model" })).toBeVisible();
  await page.getByRole("tab", { name: /Embedding/ }).click();
  await page.getByRole("button", { name: "Thêm model" }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: /Embedding/ })).toHaveAttribute("aria-pressed", "true");
  await expect(dialog.getByRole("button", { name: /Vision/ })).toBeDisabled();
  await dialog.getByRole("combobox", { name: "Nhà cung cấp", exact: true }).selectOption("mock");
  await dialog.getByLabel("Tên model").fill("mock-embed");
  await dialog.getByRole("button", { name: "Kiểm tra kết nối" }).click();
  await expect(dialog.getByRole("status")).toContainText("Vector có 8 chiều");
  await expect(dialog.getByLabel("Số chiều vector")).toHaveValue("8");
  await dialog.getByRole("button", { name: "Lưu và đóng" }).click();
  await expect(page.getByText("mock-embed", { exact: true })).toBeVisible();
  await expect(page.getByText("8 chiều")).toBeVisible();

  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "Xóa model mock-embed" }).click();
  await expect(page.getByText("mock-embed", { exact: true })).toHaveCount(0);
});

test("models: a rerank model's connection test shows its scores", async ({ page }) => {
  await page.goto("/settings/models");
  await page.getByRole("button", { name: "Thêm model" }).first().click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: /Rerank/ }).click();
  await dialog.getByRole("combobox", { name: "Nhà cung cấp", exact: true }).selectOption("mock");
  await dialog.getByLabel("Tên model").fill("mock-rerank");
  await dialog.getByRole("button", { name: "Kiểm tra kết nối" }).click();
  await expect(dialog.getByRole("status")).toContainText("0.90");
  await dialog.getByRole("button", { name: "Hủy" }).click();
  await expect(dialog).toHaveCount(0);
});

test("knowledge: chunking, index method and retrieval settings", async ({ page }) => {
  await page.goto("/settings/knowledge");
  await expect(page.getByRole("heading", { name: "Kiến thức và truy xuất" })).toBeVisible();
  await expect(page.getByText("Theo cấu trúc văn bản: Điều, Khoản (cố định)")).toBeVisible();
  await page.getByText("Cha – con", { exact: true }).click();
  await expect(page.getByLabel("Độ dài tối đa của đoạn cha")).toHaveValue("6000");
  await expect(page.getByText("Có thay đổi chưa lưu")).toBeVisible();
  await expect(page.getByRole("slider", { name: "Top K" })).toHaveValue("8");
  // No reranker on the test server: the threshold is explained, not offered.
  await expect(page.getByRole("switch", { name: /Ngưỡng điểm/ })).toBeDisabled();
});
