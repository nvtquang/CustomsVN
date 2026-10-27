import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { Force } from "@/lib/api/types";

/** The force of a document on the as-of day, in the force colours. */
export function ForceBadge({ force }: { force: Force | string }) {
  const { t } = useTranslation();
  const key = force === "" ? "expired" : force;
  const variant =
    (["in_force", "partly_expired", "not_yet_in_force", "expired"] as const).find((v) => v === key) ?? "unknown";
  return <Badge variant={variant}>{t("force." + variant)}</Badge>;
}

export function SourceBadge({ kind }: { kind: string }) {
  const { t } = useTranslation();
  return <Badge variant={kind === "consolidated" ? "accent" : "outline"}>{t("source." + kind, kind)}</Badge>;
}
