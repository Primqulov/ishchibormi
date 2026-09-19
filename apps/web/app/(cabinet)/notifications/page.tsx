"use client";
import { useState } from "react";
import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, Notification } from "@/lib/api";
import { Shell } from "@/components/Shell";
import { Bell, Check, CheckCheck, X, Briefcase, FileText, Send, AlertTriangle, MapPin } from "lucide-react";
import { T } from "@/components/T";
import { AUTH_BOT } from "@/lib/contact";
import dayjs from "dayjs";

type Filter = "all" | "application" | "system";

/** Figma "10 · Bildirishnomalar": kunlar bo'yicha guruhlangan ro'yxat + yon panel. */
export default function Notifications() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState<Filter>("all");

  const { data } = useQuery<Notification[]>({
    queryKey: ["notifications"],
    queryFn: () => api.get<Notification[]>("/api/notifications"),
  });
  const read = useMutation({
    mutationFn: () => api.post("/api/notifications/read-all"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });

  const all = data || [];
  const unread = all.filter((n) => !n.isRead).length;
  const isApp = (n: Notification) => n.relatedEntity?.type === "application";
  const items = all.filter((n) => (filter === "all" ? true : filter === "application" ? isApp(n) : !isApp(n)));

  // Kunlar bo'yicha guruhlash — Figma: "BUGUN", "KECHA", so'ng sana.
  const groups: { label: string; rows: Notification[] }[] = [];
  for (const n of items) {
    const d = dayjs(n.createdAt);
    const label = d.isSame(dayjs(), "day")
      ? "Bugun"
      : d.isSame(dayjs().subtract(1, "day"), "day")
        ? "Kecha"
        : d.format("D MMMM");
    const g = groups.find((x) => x.label === label);
    if (g) g.rows.push(n);
    else groups.push({ label, rows: [n] });
  }

  return (
    <Shell wide>
      <div className="py-6 flex flex-col gap-5">
        {/* Sarlavha */}
        <div className="flex items-start justify-between gap-3 flex-wrap">
          <div>
            <h1 className="text-[26px] font-black heading tracking-[-0.6px] leading-tight"><T>Bildirishnomalar</T></h1>
            <p className="text-[13.5px] muted mt-1">
              {unread > 0 ? <>{unread} <T>ta o'qilmagan xabar</T></> : <T>Barcha xabarlar o'qilgan</T>}
            </p>
          </div>
          <button onClick={() => read.mutate()} disabled={!unread || read.isPending} className="btn btn-outline btn-sm gap-2">
            <CheckCheck size={15} /><T>Barchasini o'qilgan deb belgilash</T>
          </button>
        </div>

        {/* Filtr tablari */}
        <div className="card p-2 flex items-center gap-1.5 overflow-x-auto">
          <Tab active={filter === "all"} onClick={() => setFilter("all")} label="Barchasi" count={all.length} />
          <Tab active={filter === "application"} onClick={() => setFilter("application")} label="Arizalar" count={all.filter(isApp).length} />
          <Tab active={filter === "system"} onClick={() => setFilter("system")} label="Tizim" count={all.filter((n) => !isApp(n)).length} />
        </div>

        <div className="grid lg:grid-cols-[1fr_300px] gap-5 items-start">
          {/* Ro'yxat */}
          <div className="flex flex-col gap-4 min-w-0">
            {items.length === 0 && (
              <div className="card p-10 text-center">
                <span className="mx-auto grid h-12 w-12 place-items-center rounded-2xl mb-3"
                      style={{ background: "var(--brand-soft)", color: "var(--brand)" }}>
                  <Bell size={20} />
                </span>
                <div className="font-bold heading"><T>Bildirishnomalar yo'q</T></div>
                <p className="mt-1 text-[13.5px] muted"><T>Yangi xabarlar shu yerda ko'rinadi.</T></p>
              </div>
            )}

            {groups.map((g) => (
              <div key={g.label} className="flex flex-col gap-2.5">
                <div className="text-[11.5px] font-bold tracking-[1px] uppercase subtle px-1"><T>{g.label}</T></div>
                {g.rows.map((n) => <Item key={n.id} n={n} />)}
              </div>
            ))}
          </div>

          {/* Yon panel */}
          <aside className="flex flex-col gap-4 lg:sticky lg:top-[92px]">
            <div className="rounded-2xl p-5" style={{ background: "var(--brand-soft)" }}>
              <h3 className="text-[14px] font-bold flex items-center gap-2" style={{ color: "var(--brand)" }}>
                <Send size={15} /><T>Telegram orqali xabar</T>
              </h3>
              <p className="mt-2 text-[12.5px] muted leading-relaxed">
                <T>Bildirishnomalarni Telegram orqali ham olishingiz mumkin — hech qanday xabarni o'tkazib yubormaysiz.</T>
              </p>
              <a href={AUTH_BOT.href} target="_blank" rel="noreferrer" className="btn btn-primary w-full mt-4 btn-sm">
                <T>Telegramni ulash</T>
              </a>
            </div>

            <div className="card p-5">
              <h3 className="section-title"><T>Eslatma</T></h3>
              <p className="mt-2 text-[12.5px] muted leading-relaxed">
                <T>Ariza holati o'zgarganda, yangi ariza kelganda va ish yakunlanganda sizga darhol xabar keladi.</T>
              </p>
            </div>
          </aside>
        </div>
      </div>
    </Shell>
  );
}

/* ── helpers ─────────────────────────────────────────── */

function Tab({ active, onClick, label, count }: { active: boolean; onClick: () => void; label: string; count: number }) {
  return (
    <button onClick={onClick} className={`navlink shrink-0 ${active ? "!bg-[color:var(--brand)] !text-white" : ""}`}>
      <T>{label}</T>
      <span className={`text-[11px] font-bold ${active ? "text-white/80" : "subtle"}`}>{count}</span>
    </button>
  );
}

const ICONS: Record<string, { icon: React.ReactNode; bg: string; fg: string }> = {
  application_accepted: { icon: <Check size={16} />,         bg: "#DFF5E5", fg: "#1A7F3C" },
  job_completed:        { icon: <Check size={16} />,         bg: "#DFF5E5", fg: "#1A7F3C" },
  new_application:      { icon: <FileText size={16} />,      bg: "#FFEED4", fg: "#8A5300" },
  application_submitted:{ icon: <FileText size={16} />,      bg: "#E5EEFF", fg: "#0038D8" },
  job_completed_request:{ icon: <AlertTriangle size={16} />, bg: "#FFEED4", fg: "#8A5300" },
  application_rejected: { icon: <X size={16} />,             bg: "#FEE4E2", fg: "#B42318" },
  application_cancelled:{ icon: <X size={16} />,             bg: "#FEE4E2", fg: "#B42318" },
  new_elon:             { icon: <Briefcase size={16} />,     bg: "#E5EEFF", fg: "#0038D8" },
  // Ish signali (backend: internal/jobalert) — hudud bo'yicha yangi e'lon.
  job_nearby:           { icon: <MapPin size={16} />,        bg: "#E5EEFF", fg: "#0038D8" },
};

function Item({ n }: { n: Notification }) {
  const c = ICONS[n.type] || { icon: <Bell size={16} />, bg: "var(--brand-soft)", fg: "var(--brand)" };
  const href = n.relatedEntity?.type === "application" ? "/process" : n.relatedEntity?.type === "elon" ? `/elon/${n.relatedEntity.id}` : "";
  const action = n.relatedEntity?.type === "application" ? "Arizani ko'rish" : n.relatedEntity?.type === "elon" ? "E'lonni ko'rish" : "";

  return (
    <div className="card p-4 flex gap-3.5" style={!n.isRead ? { borderColor: "rgba(0,56,216,0.25)" } : undefined}>
      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl" style={{ background: c.bg, color: c.fg }}>
        {c.icon}
      </span>
      <div className="flex-1 min-w-0">
        <div className="flex items-start gap-2">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-[14px] font-bold heading"><T>{n.title}</T></span>
              {!n.isRead && <span className="h-1.5 w-1.5 rounded-full shrink-0" style={{ background: "var(--brand)" }} />}
            </div>
            <p className="mt-1 text-[13px] muted leading-relaxed"><T>{n.body}</T></p>
          </div>
          <span className="text-[11.5px] subtle shrink-0">{dayjs(n.createdAt).format("HH:mm")}</span>
        </div>
        {href && (
          <Link href={href} className="btn btn-soft btn-sm mt-2.5"><T>{action}</T></Link>
        )}
      </div>
    </div>
  );
}
