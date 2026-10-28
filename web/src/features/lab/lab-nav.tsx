import { Link } from "@tanstack/react-router";
import { ClipboardCheck, FlaskConical } from "lucide-react";
import { useTranslation } from "react-i18next";

/** The two benches of the lab: retrieval and whole answers. */
export function LabNav() {
  const { t } = useTranslation();
  const cls =
    "inline-flex items-center gap-1.5 rounded-sm px-3 py-1.5 text-xs font-medium text-muted-foreground hover:text-foreground [&_svg]:size-3.5";
  return (
    <nav aria-label={t("nav.labRoom")} className="inline-flex rounded-md bg-muted p-0.5">
      <Link
        to="/lab/retrieval"
        search={{ tab: "single" }}
        className={cls}
        activeProps={{ className: "bg-surface !text-foreground shadow-xs" }}
      >
        <FlaskConical /> {t("nav.lab")}
      </Link>
      <Link to="/lab/answers" className={cls} activeProps={{ className: "bg-surface !text-foreground shadow-xs" }}>
        <ClipboardCheck /> {t("nav.answers")}
      </Link>
    </nav>
  );
}
