import { expect, test, type Route } from "@playwright/test";
import { OwnerFixture, OWNER, LISTING, capture } from "./fixture";
import { safeReturnPath } from "../../lib/auth-redirect";

class TelegramFixture extends OwnerFixture {
  onboardingCompleted = true;

  user() {
    return { id: this.userId, firstName: "Javohir", lastName: "Aliyev", phone: "+998901234567", onboardingCompleted: this.onboardingCompleted, isBlocked: false };
  }

  override async api(route: Route, path: string) {
    const request = route.request();
    const method = request.method();
    const reply = (json: unknown, status = 200) => route.fulfill({ status, json, headers: {
      "Access-Control-Allow-Origin": "http://127.0.0.1:3105", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET, POST, PATCH, OPTIONS",
    } });
    if (method === "OPTIONS") return reply({});
    if (path === "/api/auth/otp/request") return reply({ tgToken: "isolated-telegram-test" });
    if (path === "/api/auth/otp/verify") return reply({ accessToken: "isolated-web-session", user: this.user() });
    if (path === "/api/me") {
      if (method === "PATCH") this.onboardingCompleted = true;
      return reply(this.user());
    }
    if (/^\/api\/applications\/[a-f0-9]{24}$/.test(path)) {
      this.calls.push({ method, path, query: "", body: undefined });
      const app = this.applications.find((a) => path.endsWith(a.id));
      if (!app || ![app.workerId, app.employerId].includes(this.userId)) return reply({ error: { code: "not_found", message: "Ariza topilmadi." } }, 404);
      return reply(app);
    }
    return super.api(route, path);
  }
}

test("Telegram link opens the exact archived application beyond the first page", async ({ page }, info) => {
  const fixture = new TelegramFixture(506);
  const app = fixture.applications[505];
  app.workerName = "Telegram orqali ochilgan nomzod";
  app.status = "cancelled";
  app.cancelReason = "Ish vaqti o'zgargani uchun ariza bekor qilindi. Tafsilotlar to'liq ko'rinishi kerak.";
  await fixture.install(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/applications/${app.id}`);
  await expect(page.getByRole("heading", { name: "Ariza tafsilotlari", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: app.workerName, exact: true })).toBeVisible();
  await expect(page.getByText(app.cancelReason, { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Arizalarni ko'rish", exact: true })).toHaveAttribute("href", `/process?tab=employer&elon=${LISTING}`);
  await expect(page.getByRole("link", { name: "Qo'ng'iroq", exact: true })).toHaveAttribute("href", `tel:${app.workerPhone}`);
  await expect.poll(() => fixture.calls.some((c) => c.path === "/api/notifications/read" && (c.body?.relatedIds as string[])?.includes(app.id))).toBe(true);
  expect(fixture.calls.some((c) => c.path === "/api/my/elons/applications")).toBe(false);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await capture(page, info, "telegram-application-mobile");
  await fixture.assertClean();
});

test("worker detail link shows the employer and the worker's application list", async ({ page }) => {
  const fixture = new TelegramFixture(1);
  const app = fixture.applications[0];
  fixture.userId = app.workerId;
  app.ownerName = "Ish beruvchi Aliyev";
  await fixture.install(page);
  await page.goto(`/applications/${app.id}`);
  await expect(page.getByText("Siz yuborgan ariza", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: app.ownerName, exact: true })).toHaveAttribute("href", `/u/${OWNER}`);
  await expect(page.getByRole("link", { name: "Arizalarni ko'rish", exact: true })).toHaveAttribute("href", "/process");
  await expect(page.getByRole("link", { name: "Qo'ng'iroq", exact: true })).toHaveCount(0);
  await fixture.assertClean();
});

for (const onboarding of [false, true]) {
  test(`Telegram destination survives login${onboarding ? " and onboarding" : ""}`, async ({ page }) => {
    const fixture = new TelegramFixture(1);
    fixture.authenticated = false;
    fixture.onboardingCompleted = !onboarding;
    const app = fixture.applications[0];
    const destination = `/applications/${app.id}`;
    await fixture.install(page);
    await page.goto(destination);
    await expect(page).toHaveURL(new RegExp(`/login\\?next=${encodeURIComponent(destination)}$`));
    for (let i = 0; i < 6; i++) await page.getByLabel(`${i + 1}-raqam`, { exact: true }).fill(String(i + 1));
    await page.locator('label[for="consent"]').click();
    await page.getByRole("button", { name: "Davom etish", exact: true }).click();
    if (onboarding) {
      await expect(page).toHaveURL(new RegExp(`/onboarding\\?next=${encodeURIComponent(destination)}$`));
      await page.getByRole("combobox", { name: "Viloyat", exact: true }).selectOption({ index: 1 });
      await page.getByRole("combobox", { name: "Tuman", exact: true }).selectOption({ index: 1 });
      await page.getByRole("button", { name: "Saqlash va davom etish", exact: true }).click();
    }
    await expect(page).toHaveURL(new RegExp(`${destination}$`));
    await expect(page.getByRole("heading", { name: "Ariza tafsilotlari", exact: true })).toBeVisible();
    await expect(page.getByText("E'loningizga kelgan ariza", { exact: true })).toBeVisible();
    await fixture.assertClean();
  });
}

test("missing or unauthorized application link shows a useful error", async ({ page }) => {
  const fixture = new TelegramFixture(1);
  fixture.userId = "999999999999999999999999";
  await fixture.install(page);
  await page.goto(`/applications/${fixture.applications[0].id}`);
  await expect(page.getByRole("alert").filter({ hasText: "Ariza topilmadi yoki uni ko'rish huquqingiz yo'q." })).toBeVisible();
  await expect(page.getByRole("link", { name: "Qo'ng'iroq", exact: true })).toHaveCount(0);
  await fixture.assertClean();
});

test("login return URLs reject external redirects and retain internal details", () => {
  for (const value of ["https://evil.example", "//evil.example", "/\\evil.example", "/\nevil.example", "javascript:alert(1)", "/login?next=/x", "/onboarding", ""]) {
    expect(safeReturnPath(value)).toBe("/dashboard");
  }
  expect(safeReturnPath("/applications/123?tab=worker#details")).toBe("/applications/123?tab=worker#details");
});
