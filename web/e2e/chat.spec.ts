import { expect, test } from "@playwright/test";

test("asks, shows verified claims, opens a source and reports a wrong answer for review", async ({ page }) => {
  await page.goto("/");
  const box = page.locator("#question");
  await box.fill("Người khai hải quan có những quyền gì?");
  await box.press("Enter");

  // Claims arrive one by one; the invented one is dropped and counted.
  const sources = page.getByText("Căn cứ · ", { exact: false });
  await expect(sources).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText(/nhận định bị loại/)).toBeVisible();
  await expect(page).toHaveURL(/\/c\/\d+/);

  // A citation opens the chunk beside the answer, quote highlighted.
  await page
    .getByRole("button", { name: /^\[1\]/ })
    .first()
    .click();
  await expect(page.locator("mark").first()).toBeVisible();
  await page.keyboard.press("Escape");

  // Rate it down with a reason.
  await page.getByRole("button", { name: "Chưa đúng" }).click();
  const reason = page.getByPlaceholder("Điều gì chưa đúng?");
  await reason.fill("e2e: thiếu căn cứ về nghĩa vụ");
  await reason.press("Enter");
  await expect(page.getByText("Cảm ơn phản hồi của bạn")).toBeVisible();

  // The review screen has it: label it, turn it into a gold item, resolve it.
  await page.goto("/admin/feedback");
  await page
    .getByRole("button", { name: /e2e: thiếu căn cứ về nghĩa vụ/ })
    .first()
    .click();
  await expect(page.getByText("Trace của câu trả lời")).toBeVisible();
  await expect(page.getByText(/nhận định bị loại khi kiểm chứng/)).toBeVisible();
  await page.getByRole("combobox", { name: "Nhãn", exact: true }).selectOption("missing_grounds");
  await page.getByLabel("Điều khoản kỳ vọng").fill("Điều 18 Luật Hải quan 54/2014/QH13");
  await page.getByRole("button", { name: "Chuyển thành gold item" }).click();
  await expect(page.getByText(/Đã lưu\. Điều khoản: vn:law/)).toBeVisible();
  await page.getByRole("button", { name: "Đánh dấu đã xử lý" }).click();
  await expect(page.getByText("Đã lưu", { exact: true })).toBeVisible();
});

test("refuses a question outside customs law", async ({ page }) => {
  await page.goto("/");
  await page.locator("#question").fill("Giá vàng hôm nay bao nhiêu?");
  await page.locator("#question").press("Enter");
  await expect(
    page
      .getByText("Không trả lời", { exact: false })
      .or(page.getByText("ngoài phạm vi", { exact: false }))
      .first(),
  ).toBeVisible({
    timeout: 30_000,
  });
});

test("keyboard: Ctrl+K starts a new question, ? lists the shortcuts", async ({ page }) => {
  await page.goto("/hs");
  await expect(page.getByRole("heading", { name: "Tra cứu mã HS" })).toBeVisible();
  await page.keyboard.press("Control+k");
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator("#question")).toBeFocused();
  await page.locator("main").click();
  await page.keyboard.press("?");
  await expect(page.getByRole("dialog", { name: "Phím tắt" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("history: rename and delete a conversation", async ({ page }) => {
  await page.goto("/");
  await page.locator("#question").fill("Bộ hồ sơ hải quan gồm những giấy tờ nào?");
  await page.locator("#question").press("Enter");
  await expect(page).toHaveURL(/\/c\/\d+/, { timeout: 30_000 });
  const id = page.url().split("/c/")[1];
  const res = await page.request.patch("/api/conversations/" + id, { data: { title: "e2e đổi tên" } });
  expect(res.ok()).toBeTruthy();
  await page.reload();
  await expect(page.getByText("e2e đổi tên").first()).toBeVisible();
  expect((await page.request.delete("/api/conversations/" + id)).status()).toBe(204);
});
