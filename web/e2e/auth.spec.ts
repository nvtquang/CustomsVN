import { expect, test } from "@playwright/test";
import { admin, user } from "./global-setup";

test.describe("login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("refuses a wrong password, then logs in and out", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Đăng nhập" })).toBeVisible();
    await page.getByLabel("Email").fill(admin.email);
    await page.getByLabel("Mật khẩu").fill("not the password");
    await page.getByRole("button", { name: "Đăng nhập" }).click();
    await expect(page.getByRole("alert")).toContainText("Sai email hoặc mật khẩu");

    await page.getByLabel("Mật khẩu").fill(admin.password);
    await page.getByRole("button", { name: "Đăng nhập" }).click();
    await page.getByRole("button", { name: "Tài khoản và cài đặt" }).click();
    await expect(page.getByRole("menuitem", { name: "Quản trị" })).toBeVisible();

    await page.getByRole("menuitem", { name: "Đăng xuất" }).click();
    await expect(page.getByRole("button", { name: "Đăng nhập" })).toBeVisible();
    // The session is gone on the server too.
    expect((await page.request.get("/api/meta")).status()).toBe(401);
  });

  test("a user sees no admin screens", async ({ page }) => {
    await page.goto("/");
    await page.getByLabel("Email").fill(user.email);
    await page.getByLabel("Mật khẩu").fill(user.password);
    await page.getByRole("button", { name: "Đăng nhập" }).click();
    await expect(page.getByRole("link", { name: "Cuộc trò chuyện mới" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Phòng thử nghiệm" })).toHaveCount(0);
    await page.getByRole("button", { name: "Tài khoản và cài đặt" }).click();
    await expect(page.getByRole("menuitem", { name: "Quản trị" })).toHaveCount(0);
    await page.getByRole("menuitem", { name: "Cài đặt", exact: true }).click();
    await expect(page.getByRole("link", { name: "Hồ sơ và mật khẩu" }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: "Quản lý model" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Tổng quan" })).toHaveCount(0);
    await page.goto("/admin/users");
    await expect(page.getByText("Chỉ quản trị viên xem được trang này.")).toBeVisible();
    expect((await page.request.get("/api/admin/users")).status()).toBe(403);
  });
});

test("the API refuses calls without a session", async ({ playwright, baseURL }) => {
  const anon = await playwright.request.newContext({ baseURL, storageState: { cookies: [], origins: [] } });
  expect((await anon.get("/api/conversations")).status()).toBe(401);
  expect((await anon.post("/v1/chat/completions", { data: { messages: [] } })).status()).toBe(401);
  await anon.dispose();
});
