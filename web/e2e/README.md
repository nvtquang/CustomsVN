# End-to-end tests

Playwright tests of the main flows. They run against a live `customsvn serve` with a loaded corpus, and change its settings: they point the LLM roles at the fake model in `mock-llm.mjs`, lift the question limits, create the user `e2e-user@example.com` and a gold set `e2e`, and leave conversations, feedback and runs behind. **Use a test instance, never a real one.**

```sh
# 1. A test server with an admin password you know, e.g.
CUSTOMSVN_ADMIN_PASSWORD=e2e-admin-password-1 go run ./cmd/customsvn serve -listen 127.0.0.1:18080

# 2. The tests (they start the fake model on :18090 themselves)
cd web
npx playwright install chromium
E2E_ADMIN_PASSWORD=e2e-admin-password-1 E2E_MOCK_URL=http://127.0.0.1:18090/v1 npm run e2e
```

| Variable             | Default                                |                                                                                                   |
| -------------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `E2E_BASE_URL`       | `http://127.0.0.1:18080`               | the server, or a Vite dev server proxying to it (`CUSTOMSVN_API=http://127.0.0.1:18080 npx vite`) |
| `E2E_ADMIN_EMAIL`    | `admin`                                |                                                                                                   |
| `E2E_ADMIN_PASSWORD` | (required)                             |                                                                                                   |
| `E2E_MOCK_URL`       | `http://host.docker.internal:18090/v1` | where the server reaches the fake model; the default suits a server in Docker                     |

CI does not run them: it has no corpus. Run them before a release and after changing a flow they cover.
