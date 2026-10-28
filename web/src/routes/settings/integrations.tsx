import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export const Route = createFileRoute("/settings/integrations")({ component: Integrations });

function Code({ children }: { children: string }) {
  return (
    <pre className="overflow-x-auto rounded-md border bg-surface-2 p-3 font-mono text-2xs leading-relaxed">
      {children}
    </pre>
  );
}

/** How to reach CustomsVN from other tools: MCP and the OpenAI-compatible API. */
function Integrations() {
  const { t } = useTranslation();
  const origin = window.location.origin;
  const stdio = JSON.stringify(
    {
      mcpServers: {
        customsvn: {
          command: "docker",
          args: ["compose", "-f", "/path/to/CustomsVN/docker-compose.yml", "run", "--rm", "-T", "app", "mcp"],
        },
      },
    },
    null,
    2,
  );
  const http = JSON.stringify(
    { mcpServers: { customsvn: { url: origin + "/mcp", headers: { Authorization: "Bearer <CUSTOMSVN_API_TOKEN>" } } } },
    null,
    2,
  );
  const curl = `curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer <CUSTOMSVN_API_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "customsvn", "stream": false,
       "messages": [{"role": "user", "content": "Người khai hải quan có quyền gì?"}]}'`;
  const tools = [
    "search_legal",
    "get_provision",
    "resolve_reference",
    "get_document_status",
    "get_amendments",
    "lookup_hs",
    "graph_query",
    "ask",
  ];
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("integrations.title")}</h1>
        <p className="text-xs text-muted-foreground">{t("integrations.subtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("integrations.mcp")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p>{t("integrations.mcpIntro")}</p>
          <ul className="grid gap-1 text-xs sm:grid-cols-2">
            {tools.map((n) => (
              <li key={n}>
                <span className="font-mono">{n}</span> — {t("integrations.tools." + n)}
              </li>
            ))}
          </ul>
          <p className="text-xs font-medium">{t("integrations.stdio")}</p>
          <Code>{stdio}</Code>
          <p className="text-xs font-medium">{t("integrations.http")}</p>
          <Code>{http}</Code>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("integrations.openai")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p>{t("integrations.openaiIntro", { base: origin + "/v1" })}</p>
          <Code>{curl}</Code>
          <p className="text-xs text-muted-foreground">{t("integrations.openaiNote")}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("integrations.security")}</CardTitle>
        </CardHeader>
        <CardContent className="text-sm">
          <p>{t("integrations.securityText")}</p>
        </CardContent>
      </Card>
    </div>
  );
}
