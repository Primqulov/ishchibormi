"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink, Phone } from "lucide-react";
import { api, APIError, Application, User } from "@/lib/api";
import { fmtDateTime, fmtSumSom } from "@/lib/format";
import { Shell } from "@/components/Shell";
import { StatusBadge } from "@/components/StatusBadge";
import { Avatar } from "@/components/ui/Avatar";
import { T } from "@/components/T";

export default function ApplicationDetails({ params }: { params: { id: string } }) {
  const qc = useQueryClient();
  const { data: app, isPending, error, refetch } = useQuery<Application, APIError>({
    queryKey: ["application", params.id],
    queryFn: () => api.get<Application>(`/api/applications/${encodeURIComponent(params.id)}`),
    retry: false,
  });
  const { data: me } = useQuery<User>({ queryKey: ["me"], queryFn: () => api.get<User>("/api/me"), retry: false });
  const applicationId = app?.id;
  useEffect(() => {
    if (!applicationId) return;
    void api.post("/api/notifications/read", { relatedIds: [applicationId] })
      .then(() => qc.invalidateQueries({ queryKey: ["notifications"] }))
      .catch(() => { /* Reading details still works if marking read needs a retry. */ });
  }, [applicationId, qc]);
  const asEmployer = !!app && me?.id === app.employerId;
  const processURL = asEmployer && app ? `/process?tab=employer&elon=${app.elonId}` : "/process";

  return (
    <Shell>
      <div className="py-6 flex flex-col gap-5 max-w-3xl mx-auto">
        <Link href={processURL} className="inline-flex items-center gap-1.5 text-sm font-semibold w-fit" style={{ color: "var(--brand)" }}>
          <ArrowLeft size={16} /><T>Arizalarga qaytish</T>
        </Link>
        <h1 className="text-[26px] font-black heading"><T>Ariza tafsilotlari</T></h1>
        {(isPending || (app && !me)) && <div className="card p-8 text-center muted"><T>Yuklanmoqda…</T></div>}
        {error && <div role="alert" className="card p-6 flex flex-col gap-4">
          <p className="muted"><T>{["not_found", "bad_id"].includes(error.code)
            ? "Ariza topilmadi yoki uni ko'rish huquqingiz yo'q."
            : "Arizani yuklab bo'lmadi. Qayta urinib ko'ring."}</T></p>
          <button className="btn btn-outline w-fit" onClick={() => void refetch()}><T>Qayta urinish</T></button>
        </div>}
        {app && me && <div className="card p-5 sm:p-7 flex flex-col gap-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs muted mb-1"><T>{asEmployer ? "E'loningizga kelgan ariza" : "Siz yuborgan ariza"}</T></p>
              <h2 className="text-xl font-bold heading break-words">{app.elonTitle}</h2>
            </div>
            <StatusBadge status={app.status} />
          </div>
          <div className="flex items-center gap-3">
            <Avatar name={(asEmployer ? app.workerName : app.ownerName) || ""} src={asEmployer ? app.workerAvatarUrl : app.ownerAvatarUrl} />
            <div>
              <p className="text-xs muted"><T>{asEmployer ? "Ariza beruvchi" : "Ish beruvchi"}</T></p>
              <Link href={`/u/${asEmployer ? app.workerId : app.employerId}`} className="font-semibold heading">
                {(asEmployer ? app.workerName : app.ownerName) || (asEmployer ? app.workerPhone : "Ish beruvchi")}
              </Link>
            </div>
          </div>
          <dl className="grid grid-cols-1 sm:grid-cols-2 gap-5 text-sm">
            <div><dt className="muted"><T>Ish haqi</T></dt><dd className="font-semibold heading mt-1">{fmtSumSom(app.amount, app.isNegotiable)}</dd></div>
            <div><dt className="muted"><T>Kishilar soni</T></dt><dd className="font-semibold heading mt-1">{app.peopleCount || 1}</dd></div>
            <div><dt className="muted"><T>Ariza yuborilgan vaqt</T></dt><dd className="heading mt-1">{fmtDateTime(app.appliedAt)}</dd></div>
            {app.decidedAt && <div><dt className="muted"><T>Holat o'zgargan vaqt</T></dt><dd className="heading mt-1">{fmtDateTime(app.decidedAt)}</dd></div>}
            {app.completedAt && <div><dt className="muted"><T>Ish yakunlangan vaqt</T></dt><dd className="heading mt-1">{fmtDateTime(app.completedAt)}</dd></div>}
          </dl>
          {app.cancelReason && <div className="surface p-4 text-sm">
            <p className="font-semibold heading mb-1"><T>{app.status === "rejected" ? "Rad etish sababi" : "Bekor qilish sababi"}</T></p>
            <p className="muted whitespace-pre-wrap break-words">{app.cancelReason}</p>
          </div>}
          <div className="flex flex-wrap gap-3">
            <Link href={processURL} className="btn btn-primary"><T>Arizalarni ko'rish</T></Link>
            <Link href={`/elon/${app.elonId}`} className="btn btn-outline gap-1.5"><ExternalLink size={15} /><T>E'lonni ko'rish</T></Link>
            {asEmployer && app.workerPhone && <a href={`tel:${app.workerPhone.replace(/[^\d+]/g, "")}`} className="btn btn-outline gap-1.5"><Phone size={15} /><T>Qo'ng'iroq</T></a>}
          </div>
        </div>}
      </div>
    </Shell>
  );
}
