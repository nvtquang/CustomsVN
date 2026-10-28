import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Download, Upload } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AdminOnly, ErrorLine, PageTitle } from "@/features/admin/admin-only";
import { adminApi } from "@/lib/api/admin";

export const Route = createFileRoute("/settings/config")({
  component: () => (
    <AdminOnly>
      <ConfigPage />
    </AdminOnly>
  ),
});

function ConfigPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const file = useRef<HTMLInputElement>(null);
  const [parseError, setParseError] = useState("");
  const exp = useMutation({
    mutationFn: adminApi.exportConfig,
    onSuccess: (data) => {
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = "customsvn-config-" + new Date().toISOString().slice(0, 10) + ".json";
      a.click();
      URL.revokeObjectURL(a.href);
    },
  });
  const imp = useMutation({
    mutationFn: adminApi.importConfig,
    onSuccess: () => void qc.invalidateQueries(),
  });
  const pick = async (f: File | undefined) => {
    setParseError("");
    if (!f) return;
    try {
      imp.mutate(JSON.parse(await f.text()));
    } catch (e) {
      setParseError((e as Error).message);
    }
    if (file.current) file.current.value = "";
  };
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("config.title")} subtitle={t("config.subtitle")} />
      <Card>
        <CardHeader>
          <CardTitle>{t("config.export")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm">{t("config.exportText")}</p>
          <Button onClick={() => exp.mutate()} disabled={exp.isPending}>
            <Download />
            {t("config.download")}
          </Button>
          <ErrorLine error={exp.error} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("config.import")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm">{t("config.importText")}</p>
          <input
            ref={file}
            type="file"
            accept="application/json,.json"
            className="sr-only"
            id="config-file"
            onChange={(e) => void pick(e.target.files?.[0])}
          />
          <Button variant="outline" onClick={() => file.current?.click()} disabled={imp.isPending}>
            <Upload />
            {t("config.choose")}
          </Button>
          {parseError && (
            <p role="alert" className="text-sm text-destructive">
              {t("config.badFile", { msg: parseError })}
            </p>
          )}
          <ErrorLine error={imp.error} />
          {imp.data && (
            <div role="status" className="space-y-1 rounded-md border p-3 text-sm">
              <p>{t("config.applied", { parts: imp.data.applied.join(", ") || "—" })}</p>
              {imp.data.providers_without_key.length > 0 && (
                <p className="text-force-partly">
                  {t("config.needKeys", { names: imp.data.providers_without_key.join(", ") })}
                </p>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
