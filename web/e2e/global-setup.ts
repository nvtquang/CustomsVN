import { mkdirSync } from "node:fs";
import { request, type FullConfig } from "@playwright/test";

export const admin = { email: process.env.E2E_ADMIN_EMAIL ?? "admin", password: process.env.E2E_ADMIN_PASSWORD ?? "" };
export const user = { email: "e2e-user@example.com", password: "e2e-user-password-1" };

/** Where the server reaches the fake model; from inside Docker that is the host. */
const mockURL = process.env.E2E_MOCK_URL ?? "http://host.docker.internal:18090/v1";

// Logs in as the admin, points the models at the fake one, makes sure the
// test user and a small gold set exist, and saves both sessions.
export default async function globalSetup(config: FullConfig) {
  if (!admin.password) throw new Error("set E2E_ADMIN_PASSWORD to the test server's admin password");
  const baseURL = config.projects[0].use.baseURL!;
  mkdirSync("e2e/.auth", { recursive: true });
  const a = await request.newContext({ baseURL, extraHTTPHeaders: { Origin: baseURL } });
  const login = await a.post("/api/auth/login", { data: admin });
  if (!login.ok()) throw new Error("admin login failed: " + (await login.text()));
  const llm = await a.put("/api/settings/llm", {
    data: {
      providers: [{ name: "mock", kind: "openai", base_url: mockURL, api_key_env: "", models: ["mock-1"] }],
      roles: { answer: [{ provider: "mock", model: "mock-1" }], router: [{ provider: "mock", model: "mock-1" }] },
      prices: { "mock/mock-1": { input: 3, output: 15 } },
      master_key: false,
    },
  });
  if (!llm.ok()) throw new Error("llm settings: " + (await llm.text()));
  // Generous limits: the suite asks many questions in a row.
  await a.put("/api/settings/limits", { data: { questions_per_minute: 0, questions_per_day: 0 } });
  const created = await a.post("/api/admin/users", { data: { ...user, name: "E2E", role: "user" } });
  if (!created.ok()) {
    // It exists from an earlier run: reset its password to the known one.
    const users = (await (await a.get("/api/admin/users")).json()) as { id: number; email: string }[];
    const u = users.find((x) => x.email === user.email);
    if (!u) throw new Error("cannot create the test user: " + (await created.text()));
    await a.patch("/api/admin/users/" + u.id, { data: { password: user.password, disabled: false, role: "user" } });
  }
  const sets = (await (await a.get("/api/goldsets")).json()) as { name: string }[];
  if (!sets.some((s) => s.name === "e2e")) {
    const csv =
      "id,question,expected_provisions,expected_answer,as_of_date,category,should_refuse,notes\n" +
      "e1,Người khai hải quan có những quyền gì?,Điều 18 Luật Hải quan 54/2014/QH13,,,quyen,false,\n" +
      "e2,Giá vàng hôm nay bao nhiêu?,,,,ngoai-pham-vi,true,\n";
    const imp = await a.post("/api/goldsets", { data: { name: "e2e", format: "csv", content: csv, dry_run: false } });
    if (!imp.ok()) throw new Error("gold set: " + (await imp.text()));
  }
  await a.storageState({ path: "e2e/.auth/admin.json" });
  await a.dispose();

  const u = await request.newContext({ baseURL, extraHTTPHeaders: { Origin: baseURL } });
  const ul = await u.post("/api/auth/login", { data: user });
  if (!ul.ok()) throw new Error("user login failed: " + (await ul.text()));
  await u.storageState({ path: "e2e/.auth/user.json" });
  await u.dispose();
}
