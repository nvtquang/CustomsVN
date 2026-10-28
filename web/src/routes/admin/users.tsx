import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { KeyRound, Save, Trash2, UserPlus } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, Input, Select } from "@/components/ui/form";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { when } from "@/lib/utils";
import { useMe } from "@/features/auth/use-me";
import { adminApi, type Limits, type User } from "@/lib/api/admin";

export const Route = createFileRoute("/admin/users")({
  component: () => (
    <AdminOnly>
      <UsersPage />
    </AdminOnly>
  ),
});

function UsersPage() {
  const { t } = useTranslation();
  const users = useQuery({ queryKey: ["admin", "users"], queryFn: adminApi.users });
  return (
    <div className="mx-auto max-w-5xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("users.title")} subtitle={t("users.subtitle")} />
      <NewUser />
      <Card>
        <CardHeader>
          <CardTitle>{t("users.list")}</CardTitle>
        </CardHeader>
        <CardContent>
          <ErrorLine error={users.error} />
          {users.isPending ? <Skeleton /> : <UserTable users={users.data ?? []} />}
        </CardContent>
      </Card>
      <LimitsCard />
    </div>
  );
}

function NewUser() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState("user");
  const [password, setPassword] = useState("");
  const create = useMutation({
    mutationFn: () => adminApi.createUser({ email: email.trim(), name: name.trim(), role, password }),
    onSuccess: () => {
      setEmail("");
      setName("");
      setPassword("");
      void qc.invalidateQueries({ queryKey: ["admin", "users"] });
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (email.trim()) create.mutate();
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("users.add")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <form className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5 lg:items-end" onSubmit={submit}>
          <Field label={t("auth.email")} className="lg:col-span-2">
            <Input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <Field label={t("users.name")}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label={t("users.role")}>
            <Select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="user">{t("auth.roles.user")}</option>
              <option value="admin">{t("auth.roles.admin")}</option>
            </Select>
          </Field>
          <Field label={t("users.password")}>
            <Input
              type="password"
              autoComplete="new-password"
              placeholder={t("users.passwordGenerated")}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <div className="lg:col-span-5">
            <Button type="submit" disabled={create.isPending}>
              <UserPlus />
              {t("users.create")}
            </Button>
          </div>
        </form>
        <ErrorLine error={create.error} />
        {create.data?.password && (
          <p role="status" className="rounded-md border border-primary/40 bg-accent p-3 text-sm">
            {t("users.generated", { email: create.data.user.email })}{" "}
            <code className="rounded bg-surface px-1.5 py-0.5 font-mono select-all">{create.data.password}</code>
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function UserTable({ users }: { users: User[] }) {
  const { t } = useTranslation();
  return (
    <Table>
      <thead>
        <tr>
          <Th>{t("auth.email")}</Th>
          <Th>{t("users.name")}</Th>
          <Th>{t("users.role")}</Th>
          <Th>{t("users.lastLogin")}</Th>
          <Th>{t("users.status")}</Th>
          <Th className="text-right">{t("users.actions")}</Th>
        </tr>
      </thead>
      <tbody>
        {users.map((u) => (
          <UserRow key={u.id} user={u} />
        ))}
      </tbody>
    </Table>
  );
}

function UserRow({ user }: { user: User }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const me = useMe().data;
  const [reset, setReset] = useState<string | null>(null);
  const done = () => void qc.invalidateQueries({ queryKey: ["admin", "users"] });
  const update = useMutation({
    mutationFn: (body: { role?: string; disabled?: boolean; password?: string }) => adminApi.updateUser(user.id, body),
    onSuccess: done,
  });
  const remove = useMutation({ mutationFn: () => adminApi.deleteUser(user.id), onSuccess: done });
  const self = me?.id === user.id;
  const resetPassword = () => {
    // A random password the admin hands over; the user changes it after.
    const bytes = crypto.getRandomValues(new Uint8Array(12));
    const pw = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, "x");
    update.mutate({ password: pw }, { onSuccess: () => setReset(pw) });
  };
  return (
    <Tr>
      <Td className="font-medium">
        {user.email}
        {self && <span className="ml-1 text-muted-foreground">({t("users.you")})</span>}
        {reset && (
          <div className="mt-1 text-2xs" role="status">
            {t("users.newPassword")} <code className="font-mono select-all">{reset}</code>
          </div>
        )}
      </Td>
      <Td>{user.name || "—"}</Td>
      <Td>
        <Select
          aria-label={t("users.role")}
          className="h-7 w-32 text-xs"
          value={user.role}
          disabled={self || update.isPending}
          onChange={(e) => update.mutate({ role: e.target.value })}
        >
          <option value="user">{t("auth.roles.user")}</option>
          <option value="admin">{t("auth.roles.admin")}</option>
        </Select>
      </Td>
      <Td className="whitespace-nowrap">{when(user.last_login)}</Td>
      <Td>
        {user.disabled ? (
          <Badge variant="expired">{t("users.disabled")}</Badge>
        ) : (
          <Badge variant="in_force">{t("users.active")}</Badge>
        )}
      </Td>
      <Td className="text-right whitespace-nowrap">
        <div className="inline-flex gap-1">
          <Button
            size="sm"
            variant="outline"
            disabled={self || update.isPending}
            onClick={() => update.mutate({ disabled: !user.disabled })}
          >
            {user.disabled ? t("users.enable") : t("users.disable")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={update.isPending}
            onClick={resetPassword}
            title={t("users.reset")}
          >
            <KeyRound />
            <span className="sr-only">{t("users.reset")}</span>
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={self || remove.isPending}
            title={t("users.delete")}
            onClick={() => {
              if (window.confirm(t("users.confirmDelete", { email: user.email }))) remove.mutate();
            }}
          >
            <Trash2 />
            <span className="sr-only">{t("users.delete")}</span>
          </Button>
        </div>
        <ErrorLine error={update.error ?? remove.error} />
      </Td>
    </Tr>
  );
}

function LimitsCard() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["admin", "limits"], queryFn: adminApi.limits });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("limits.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">{t("limits.subtitle")}</p>
        {q.data ? <LimitsForm initial={q.data} /> : <Skeleton rows={1} />}
      </CardContent>
    </Card>
  );
}

function LimitsForm({ initial }: { initial: Limits }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [l, setL] = useState(initial);
  const save = useMutation({
    mutationFn: () => adminApi.saveLimits(l),
    onSuccess: (v) => qc.setQueryData(["admin", "limits"], v),
  });
  return (
    <form
      className="flex flex-wrap items-end gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <Field label={t("limits.perMinute")}>
        <Input
          type="number"
          min={0}
          max={1000}
          className="w-36"
          value={l.questions_per_minute}
          onChange={(e) => setL({ ...l, questions_per_minute: Number(e.target.value) })}
        />
      </Field>
      <Field label={t("limits.perDay")}>
        <Input
          type="number"
          min={0}
          max={100000}
          className="w-36"
          value={l.questions_per_day}
          onChange={(e) => setL({ ...l, questions_per_day: Number(e.target.value) })}
        />
      </Field>
      <Button type="submit" disabled={save.isPending}>
        <Save />
        {t("common.save")}
      </Button>
      {save.isSuccess && <span className="text-xs text-primary">{t("common.saved")}</span>}
      <ErrorLine error={save.error} />
    </form>
  );
}
