import { useMutation } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { KeyRound } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/form";
import { useMe } from "@/features/auth/use-me";
import { authApi } from "@/lib/api/admin";

export const Route = createFileRoute("/settings/account")({ component: AccountPage });

function AccountPage() {
  const { t } = useTranslation();
  const me = useMe().data;
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const change = useMutation({
    mutationFn: () => authApi.changePassword(current, next),
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setConfirm("");
    },
  });
  const mismatch = confirm !== "" && confirm !== next;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!mismatch && next.length >= 10) change.mutate();
  };
  return (
    <div className="mx-auto max-w-xl space-y-4 p-4 lg:p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("account.title")}</h1>
        {me && (
          <p className="text-xs text-muted-foreground">
            {me.email} · {t("auth.roles." + me.role)}
          </p>
        )}
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("account.changePassword")}</CardTitle>
        </CardHeader>
        <CardContent>
          <form className="space-y-3" onSubmit={submit}>
            <Field label={t("account.current")}>
              <Input
                type="password"
                autoComplete="current-password"
                required
                value={current}
                onChange={(e) => setCurrent(e.target.value)}
              />
            </Field>
            <Field label={t("account.new")}>
              <Input
                type="password"
                autoComplete="new-password"
                required
                minLength={10}
                value={next}
                onChange={(e) => setNext(e.target.value)}
                aria-describedby="pw-hint"
              />
            </Field>
            <p id="pw-hint" className="text-2xs text-muted-foreground">
              {t("account.minLength")}
            </p>
            <Field label={t("account.confirm")}>
              <Input
                type="password"
                autoComplete="new-password"
                required
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                aria-invalid={mismatch}
              />
            </Field>
            {mismatch && <p className="text-sm text-destructive">{t("account.mismatch")}</p>}
            {change.error && (
              <p role="alert" className="text-sm text-destructive">
                {t("common.error", { msg: change.error.message })}
              </p>
            )}
            {change.isSuccess && (
              <p role="status" className="text-sm text-primary">
                {t("account.saved")}
              </p>
            )}
            <Button type="submit" disabled={change.isPending || mismatch || next.length < 10}>
              <KeyRound />
              {t("account.changePassword")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
