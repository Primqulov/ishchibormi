"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { api, User } from "@/lib/api";
import { returnPath } from "@/lib/auth-redirect";
import { Check, CheckCircle2 } from "lucide-react";
import { AvatarUploader } from "@/components/ui/ImageUpload";
import { LangMenu } from "@/components/LangMenu";
import { ThemeToggle } from "@/components/ThemeToggle";
import { T, useT } from "@/components/T";
import { PROFILE_REGIONS, districtsOf, optionsWithCurrent } from "@/lib/regions";

/** Figma "02 · Shaxsiy ma'lumotlarni kiritish": chapda qadamlar, o'ngda forma. */
export default function Onboarding() {
  const router = useRouter();
  const qc = useQueryClient();
  const t = useT();
  const [me, setMe] = useState<User | null>(null);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [region, setRegion] = useState("");
  const [district, setDistrict] = useState("");
  const [avatarUrl, setAvatarUrl] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const regionOptions = optionsWithCurrent(PROFILE_REGIONS, region);
  const districtOptions = optionsWithCurrent(districtsOf(region), district);

  useEffect(() => {
    api.get<User>("/api/me").then((u) => {
      setMe(u);
      setFirstName(u.firstName || "");
      setLastName(u.lastName || "");
      setRegion(u.region || "");
      setDistrict(u.district || "");
      setAvatarUrl(u.avatarUrl || undefined);
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      // Yangilangan foydalanuvchini React Query keshiga yozamiz, aks holda
      // /dashboard'dagi Shell eski (onboardingCompleted=false) keshni o'qib,
      // foydalanuvchini yana /onboarding'ga qaytaradi (ikki marta saqlash muammosi).
      const updated = await api.patch<User>("/api/me", {
        firstName, lastName, region, district, avatarUrl: avatarUrl || "",
      });
      qc.setQueryData(["me"], updated);
      router.replace(returnPath());
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="min-h-screen grid lg:grid-cols-[36%_1fr]">
      {/* ── Chap panel — qadamlar ────────────────────────────────── */}
      <aside className="gradient-hero text-white hidden lg:flex flex-col gap-10 p-12">
        <Link href="/" className="text-[21px] font-black tracking-[-0.3px]">
          <span className="text-white">Ishchi</span><span style={{ color: "var(--accent)" }}>Bormi</span>
        </Link>
        <ol className="flex flex-col gap-6">
          <StepItem done n={1} title="Telegram kodi" body="Tasdiqlandi" />
          <StepItem active n={2} title="Shaxsiy ma'lumotlar" body="Hozir to'ldiring" />
          <StepItem n={3} title="Tayyor" body="Ish qidiring yoki e'lon bering" />
        </ol>
      </aside>

      {/* ── O'ng panel — forma ───────────────────────────────────── */}
      <div className="flex flex-col" style={{ background: "var(--bg-subtle)" }}>
        <header className="flex items-center justify-end gap-2 px-5 sm:px-10 py-5">
          <LangMenu />
          <ThemeToggle />
        </header>

        <main className="flex-1 grid place-items-center px-5 sm:px-10 pb-10">
          <form onSubmit={submit} className="card w-full max-w-[620px] p-7 sm:p-8 animate-slide-up">
            <h1 className="text-[24px] font-black heading tracking-[-0.6px]"><T>Shaxsiy ma'lumotlar</T></h1>
            <p className="mt-1.5 text-[13.5px] muted">
              <T>Bu ma'lumotlar e'lon beruvchilarga ko'rinadi va ishonchni oshiradi.</T>
            </p>

            <div className="mt-6 surface p-4">
              <AvatarUploader value={avatarUrl} name={`${firstName} ${lastName}`} onChange={setAvatarUrl} />
            </div>

            <div className="mt-5 grid sm:grid-cols-2 gap-4">
              <label className="block">
                <span className="text-[13px] font-bold heading"><T>Ism</T></span>
                <input className="input mt-1.5" value={firstName} onChange={(e) => setFirstName(e.target.value)} placeholder={t("Ism")} required />
              </label>
              <label className="block">
                <span className="text-[13px] font-bold heading"><T>Familiya</T></span>
                <input className="input mt-1.5" value={lastName} onChange={(e) => setLastName(e.target.value)} placeholder={t("Familiya")} />
              </label>
            </div>

            <label className="block mt-4">
              <span className="text-[13px] font-bold heading"><T>Telefon raqam</T></span>
              <div className="mt-1.5 flex items-center gap-2">
                <input className="input" value={me?.phone || ""} disabled />
                <span className="badge-success shrink-0"><CheckCircle2 size={12} /> <T>Tasdiqlangan</T></span>
              </div>
            </label>

            <div className="mt-4 grid sm:grid-cols-2 gap-4">
              <label className="block">
                <span className="text-[13px] font-bold heading"><T>Viloyat</T></span>
                <select className="input mt-1.5" value={region} onChange={(e) => {
                  setRegion(e.target.value);
                  setDistrict("");
                }} required>
                  <option value="">{t("Tanlang")}</option>
                  {regionOptions.map((r) => <option key={r} value={r}>{r}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-[13px] font-bold heading"><T>Tuman</T></span>
                <select className="input mt-1.5" value={district}
                        onChange={(e) => setDistrict(e.target.value)} disabled={!region} required>
                  <option value="">{t("Tanlang")}</option>
                  {districtOptions.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
              </label>
            </div>

            <button disabled={saving} className="btn btn-primary w-full mt-7 !py-3.5 disabled:opacity-50">
              {saving ? <T>Saqlanmoqda…</T> : <T>Saqlash va davom etish</T>}
            </button>
            <button type="button" onClick={() => router.replace("/dashboard")}
                    className="w-full mt-3 text-[13px] font-semibold subtle hover:text-[color:var(--text)] transition">
              <T>Keyinroq to'ldirish</T>
            </button>
          </form>
        </main>
      </div>
    </div>
  );
}

function StepItem({ n, title, body, done, active }: { n: number; title: string; body: string; done?: boolean; active?: boolean }) {
  return (
    <li className="flex items-center gap-3.5">
      <span
        className="grid h-8 w-8 shrink-0 place-items-center rounded-full text-[13px] font-bold"
        style={
          done
            ? { background: "#fff", color: "var(--brand)" }
            : active
              ? { background: "rgba(255,255,255,0.28)", color: "#fff" }
              : { background: "rgba(255,255,255,0.12)", color: "rgba(255,255,255,0.65)" }
        }
      >
        {done ? <Check size={15} /> : n}
      </span>
      <div>
        <div className={`text-[14px] font-bold ${active || done ? "text-white" : "text-white/60"}`}><T>{title}</T></div>
        <div className="text-[12px] text-white/60"><T>{body}</T></div>
      </div>
    </li>
  );
}
