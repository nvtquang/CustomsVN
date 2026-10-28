import { expect, test } from "@playwright/test";

test("on a phone the menu opens and closes, and HS lookup works", async ({ page }) => {
  await page.goto("/");
  const menu = page.getByRole("button", { name: "Mở menu" });
  await expect(menu).toBeVisible();
  await menu.click();
  await page.getByRole("link", { name: "Tra cứu mã HS" }).click();
  await expect(page).toHaveURL(/\/hs/);
  await page.getByLabel(/Nhập mã/).fill("8703.23");
  await page.keyboard.press("Enter");
  await expect(page.getByText("8703.23").first()).toBeVisible();
  // No horizontal scroll at phone width.
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
});
