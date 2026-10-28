import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, LogIn } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/form";
import { authApi } from "@/lib/api/admin";
import { ApiError } from "@/lib/api/client";

/** Shown instead of the app until someone logs in. */
export function LoginScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: () => authApi.login(email.trim(), password),
    onSuccess: (user) => {
      // Whatever was cached belonged to nobody or to the previous user. The
      // "me" query stays: the app shell is watching it.
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== "me" });
      qc.setQueryData(["me"], user);
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (email.trim() && password) login.mutate();
  };
  let error = "";
  if (login.error instanceof ApiError) {
    const body = login.error.body as { code?: string; retry_after?: number } | null;
    error =
      body?.code === "rate_limit"
        ? t("auth.tooMany", { s: body.retry_after ?? 60 })
        : login.error.status === 401
          ? t("auth.wrong")
          : t("common.error", { msg: login.error.message });
  } else if (login.error) {
    error = t("common.error", { msg: login.error.message });
  }
  return (
    <main className="flex min-h-dvh items-center justify-center bg-background p-4">
      <Card className="w-full max-w-sm">
        <CardContent className="space-y-5 p-6">
          <div>
            <h1 className="text-xl font-semibold text-primary">{t("app.name")}</h1>
            <p className="text-xs text-muted-foreground">{t("app.tagline")}</p>
          </div>
          <form className="space-y-3" onSubmit={submit} aria-describedby={error ? "login-error" : undefined}>
            <Field label={t("auth.email")}>
              <Input
                name="email"
                autoComplete="username"
                autoFocus
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </Field>
            <Field label={t("auth.password")}>
              <Input
                name="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            {error && (
              <p id="login-error" role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full" disabled={login.isPending}>
              {login.isPending ? <Loader2 className="animate-spin" /> : <LogIn />}
              {t("auth.submit")}
            </Button>
          </form>
          <p className="text-2xs text-muted-foreground">{t("auth.hint")}</p>
        </CardContent>
      </Card>
    </main>
  );
}
