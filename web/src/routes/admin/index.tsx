import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Select } from "@/components/ui/form";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { BarChart } from "@/features/admin/bar-chart";
import { adminApi, type DayStats } from "@/lib/api/admin";

export const Route = createFileRoute("/admin/")({
  component: () => (
    <AdminOnly>
      <Dashboard />
    </AdminOnly>
  ),
});

const pct = (a: number, b: number) => (b > 0 ? Math.round((a / b) * 1000) / 10 + "%" : "—");
const ms = (v: number) => (v > 0 ? (v / 1000).toFixed(2) + " s" : "—");
const usd = (v: number) => "$" + v.toFixed(v < 1 ? 4 : 2);

function Dashboard() {
  const { t } = useTranslation();
  const [days, setDays] = useState(30);
  const [table, setTable] = useState(false);
  const q = useQuery({ queryKey: ["admin", "dashboard", days], queryFn: () => adminApi.dashboard(days) });
  return (
    <div className="mx-auto max-w-6xl space-y-4 p-4 lg:p-6">
      <PageTitle
        title={t("dash.title")}
        subtitle={t("dash.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            <Checkbox label={t("dash.tableView")} checked={table} onChange={(e) => setTable(e.target.checked)} />
            <Select
              aria-label={t("dash.range")}
              className="h-8 w-44 text-xs"
              value={days}
              onChange={(e) => setDays(Number(e.target.value))}
            >
              {[7, 30, 90].map((d) => (
                <option key={d} value={d}>
                  {t("dash.lastDays", { n: d })}
                </option>
              ))}
            </Select>
          </div>
        }
      />
      <ErrorLine error={q.error} />
      {q.isPending ? (
        <Skeleton rows={6} />
      ) : (
        q.data && (
          <>
            <Tiles total={q.data.total} openDown={q.data.open_down} />
            {q.data.unpriced.length > 0 && (
              <p className="text-xs text-force-partly">
                {t("dash.unpriced", { models: q.data.unpriced.join(", ") })}{" "}
                <Link to="/settings/models" className="underline">
                  {t("nav.llm")}
                </Link>
              </p>
            )}
            {table ? <DayTable days={q.data.days} /> : <Charts days={q.data.days} />}
            <p className="text-2xs text-muted-foreground">{t("dash.zone", { zone: q.data.zone })}</p>
          </>
        )
      )}
    </div>
  );
}

function Tile({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="rounded-lg border bg-surface p-3">
      <div className="text-2xs text-muted-foreground">{label}</div>
      <div className="text-2xl font-semibold tabular-nums">{value}</div>
      {note && <div className="text-2xs text-muted-foreground">{note}</div>}
    </div>
  );
}

function Tiles({ total, openDown }: { total: DayStats; openDown: number }) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
      <Tile
        label={t("dash.questions")}
        value={total.questions.toLocaleString()}
        note={t("dash.users", { n: total.users })}
      />
      <Tile
        label={t("dash.refusalRate")}
        value={pct(total.refused, total.answers)}
        note={t("dash.ofAnswers", { n: total.answers })}
      />
      <Tile
        label={t("dash.firstClaimP50")}
        value={ms(total.first_claim_p50_ms)}
        note={t("dash.p95", { v: ms(total.first_claim_p95_ms) })}
      />
      <Tile
        label={t("dash.cost")}
        value={usd(total.cost_usd)}
        note={t("dash.evalCost", { v: usd(total.eval_cost_usd) })}
      />
      <Tile label={t("dash.feedback")} value={`👍 ${total.up} · 👎 ${total.down}`} />
      <Link
        to="/admin/feedback"
        search={{ status: "open", rating: -1 }}
        className="rounded-lg focus-visible:outline-2 focus-visible:outline-ring"
      >
        <Tile label={t("dash.openDown")} value={String(openDown)} note={t("dash.toReview")} />
      </Link>
    </div>
  );
}

function Charts({ days }: { days: DayStats[] }) {
  const { t } = useTranslation();
  const d = days.map((x) => x.day);
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardContent className="pt-4">
          <BarChart
            title={t("dash.questionsPerDay")}
            days={d}
            values={days.map((x) => x.questions)}
            format={(v) => String(v)}
          />
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-4">
          <BarChart
            title={t("dash.refusalPerDay")}
            days={d}
            values={days.map((x) => (x.answers ? (x.refused / x.answers) * 100 : 0))}
            format={(v) => v.toFixed(1) + "%"}
          />
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-4">
          <BarChart
            title={t("dash.latencyPerDay")}
            days={d}
            values={days.map((x) => x.first_claim_p95_ms)}
            format={ms}
          />
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-4">
          <BarChart
            title={t("dash.costPerDay")}
            days={d}
            values={days.map((x) => x.cost_usd + x.eval_cost_usd)}
            format={usd}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function DayTable({ days }: { days: DayStats[] }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("dash.tableView")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <thead>
            <tr>
              <Th>{t("dash.day")}</Th>
              <Th className="text-right">{t("dash.questions")}</Th>
              <Th className="text-right">{t("dash.refusalRate")}</Th>
              <Th className="text-right">{t("dash.clarified")}</Th>
              <Th className="text-right">{t("dash.errors")}</Th>
              <Th className="text-right">{t("dash.firstClaimP50")}</Th>
              <Th className="text-right">p95</Th>
              <Th className="text-right">{t("dash.cost")}</Th>
              <Th className="text-right">{t("dash.evalCostShort")}</Th>
              <Th className="text-right">👍/👎</Th>
            </tr>
          </thead>
          <tbody>
            {[...days].reverse().map((x) => (
              <Tr key={x.day}>
                <Td className="whitespace-nowrap">{x.day}</Td>
                <Td className="text-right tabular-nums">{x.questions}</Td>
                <Td className="text-right tabular-nums">{pct(x.refused, x.answers)}</Td>
                <Td className="text-right tabular-nums">{x.clarified}</Td>
                <Td className="text-right tabular-nums">{x.errors}</Td>
                <Td className="text-right tabular-nums">{ms(x.first_claim_p50_ms)}</Td>
                <Td className="text-right tabular-nums">{ms(x.first_claim_p95_ms)}</Td>
                <Td className="text-right tabular-nums">{usd(x.cost_usd)}</Td>
                <Td className="text-right tabular-nums">{usd(x.eval_cost_usd)}</Td>
                <Td className="text-right tabular-nums">
                  {x.up}/{x.down}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </CardContent>
    </Card>
  );
}
