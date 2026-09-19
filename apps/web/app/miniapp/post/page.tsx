"use client";

import Script from "next/script";
import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { CheckCircle2, Loader2, X } from "lucide-react";
import { api, APIError, Elon, setAccess, User } from "@/lib/api";
import { PROFILE_REGIONS, districtsOf, optionsWithCurrent } from "@/lib/regions";
import { CreateElonForm } from "@/components/CreateElonForm";
import { TelegramWebApp } from "@/lib/miniapp";

type Session = { accessToken: string; user: User; botUsername: string };

export default function MiniAppPost() {
  const app = useRef<TelegramWebApp>();
  const started = useRef(false);
  const qc = useQueryClient();
  const { setTheme } = useTheme();
  const [stage, setStage] = useState<"loading" | "contact" | "profile" | "form" | "success" | "error">("loading");
  const [me, setMe] = useState<User>();
  const [botUsername, setBotUsername] = useState("");
  const [created, setCreated] = useState<Elon>();
  const [formKey, setFormKey] = useState(0);
  const [consent, setConsent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function signIn(contactData?: string) {
    setError("");
    try {
      const session = await api.post<Session>("/api/auth/miniapp/session", { initData: app.current?.initData, contactData, consent }, { auth: "none" });
      setAccess(session.accessToken);
      qc.clear();
      setMe(session.user);
      setBotUsername(session.botUsername);
      setStage(session.user.firstName && session.user.region && session.user.district ? "form" : "profile");
    } catch (err) {
      const failure = err as APIError;
      if (failure.code === "contact_required") setStage("contact");
      else { setError(failure.message || "Mini App'ni ochib bo'lmadi. Qayta urinib ko'ring."); setStage("error"); }
    }
  }

  function start() {
    if (started.current) return;
    started.current = true;
    const telegram = window.Telegram?.WebApp;
    if (!telegram?.initData) {
      setError("Bu sahifani Telegram botidagi «E'lon berish» tugmasi orqali oching.");
      setStage("error");
      return;
    }
    app.current = telegram;
    telegram.ready();
    telegram.expand();
    setTheme(telegram.colorScheme);
    setAccess(null);
    void signIn();
  }

  useEffect(() => {
    const telegram = app.current;
    if (!telegram) return;
    if (stage === "form") telegram.enableClosingConfirmation();
    else telegram.disableClosingConfirmation();
    const syncTheme = () => setTheme(telegram.colorScheme);
    telegram.onEvent("themeChanged", syncTheme);
    return () => telegram.offEvent("themeChanged", syncTheme);
  }, [stage, setTheme]);

  function shareContact() {
    if (!consent || busy || !app.current) return;
    setBusy(true); setError("");
    try {
      app.current.requestContact((sent, event) => {
        if (!sent || !event?.response) {
          setBusy(false);
          setError(sent ? "Telefonni tasdiqlab bo'lmadi. Qayta urinib ko'ring." : "Davom etish uchun telefon raqamingizni ulashing.");
          return;
        }
        void signIn(event.response).finally(() => setBusy(false));
      });
    } catch {
      setBusy(false);
      setError("Telefonni ulashish uchun Telegram ilovasini yangilang va qayta urinib ko'ring.");
    }
  }

  return <>
    <Script src="https://telegram.org/js/telegram-web-app.js" strategy="afterInteractive" onReady={start} onError={() => { setError("Telegram bilan bog'lanib bo'lmadi. Internetni tekshirib, qayta oching."); setStage("error"); }} />
    <header className="flex items-center justify-between border-b px-4 py-3 bg-[color:var(--card)]">
      <span className="font-black heading">Ishchi Bormi</span>
      <button type="button" className="btn btn-ghost btn-sm" aria-label="Yopish" onClick={() => app.current?.close()}><X size={20} /></button>
    </header>
    {stage === "loading" && <div role="status" className="grid justify-items-center gap-3 px-5 py-16"><Loader2 className="animate-spin" /><p>Telegram orqali kirilmoqda…</p></div>}
    {stage === "error" && <section className="card m-4 p-6"><h1 className="text-xl font-bold mb-3">E&apos;lon berish</h1><p role="alert">{error}</p>{app.current && <button className="btn btn-primary mt-5" onClick={() => { setStage("loading"); void signIn(); }}>Qayta urinish</button>}</section>}
    {stage === "contact" && <section className="card m-4 p-6">
      <h1 className="text-xl font-bold">Telefon raqamingizni tasdiqlang</h1>
      <p className="muted text-sm mt-2">E&apos;loningizni hisobingizga bog&apos;lash uchun Telegram orqali o&apos;z raqamingizni ulashing.</p>
      <label className="flex items-start gap-3 text-sm mt-6"><input type="checkbox" checked={consent} onChange={(event) => setConsent(event.target.checked)} className="mt-1 h-4 w-4 shrink-0" /><span><a className="underline" href="https://ishchibormi.uz/foydalanish-shartlari" target="_blank" rel="noreferrer">Foydalanish shartlari</a> va <a className="underline" href="https://ishchibormi.uz/maxfiylik-siyosati" target="_blank" rel="noreferrer">maxfiylik siyosati</a> bilan tanishdim va roziman.</span></label>
      {error && <p role="alert" className="text-danger text-sm mt-3">{error}</p>}
      <button className="btn btn-primary w-full mt-5" disabled={!consent || busy} onClick={shareContact}>{busy ? "Tasdiqlanmoqda…" : "Telefon raqamni ulashish"}</button>
    </section>}
    {stage === "profile" && me && <MiniAppProfile user={me} suggestedName={app.current?.initDataUnsafe?.user?.first_name} onSaved={(user) => { setMe(user); setStage("form"); }} />}
    {stage === "form" && <CreateElonForm key={formKey} embedded onCancel={() => app.current?.close()} onCreated={(job) => { setCreated(job); setStage("success"); app.current?.disableClosingConfirmation(); }} />}
    {stage === "success" && created && <section className="card m-4 p-7 text-center">
      <CheckCircle2 className="mx-auto text-success" size={54} />
      <h1 className="font-bold text-xl mt-4">E&apos;loningiz joylashtirildi!</h1>
      <p className="muted mt-2">{created.title}</p><p className="text-sm muted mt-1">Endi ishchilar arizalarini kuting.</p>
      {/^[A-Za-z][A-Za-z0-9_]{4,31}$/.test(botUsername) && <button className="btn btn-primary w-full mt-6" onClick={() => app.current?.openTelegramLink(`https://t.me/${botUsername}?start=job_${created.id}`)}>Botda e&apos;lonni ko&apos;rish</button>}
      <button className="btn btn-outline w-full mt-3" onClick={() => { setCreated(undefined); setFormKey((key) => key + 1); setStage("form"); }}>Yana e&apos;lon berish</button>
      <button className="btn btn-ghost w-full mt-2" onClick={() => app.current?.close()}>Yopish</button>
    </section>}
  </>;
}

function MiniAppProfile({ user, suggestedName, onSaved }: { user: User; suggestedName?: string; onSaved: (user: User) => void }) {
  const [firstName, setFirstName] = useState(user.firstName || suggestedName || "");
  const [lastName, setLastName] = useState(user.lastName || "");
  const [region, setRegion] = useState(user.region || "");
  const [district, setDistrict] = useState(user.district || "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <form className="card m-4 p-6 grid gap-4" onSubmit={async (event) => {
    event.preventDefault(); setBusy(true); setError("");
    try { await api.patch("/api/me", { firstName: firstName.trim(), lastName: lastName.trim(), region, district }); onSaved(await api.get<User>("/api/me")); }
    catch (err) { setError((err as APIError).message || "Profil saqlanmadi. Qayta urinib ko'ring."); }
    finally { setBusy(false); }
  }}>
    <h1 className="text-xl font-bold">Profilni to&apos;ldiring</h1><p className="muted text-sm">Bu ma&apos;lumotlar ishchilarga sizni tanishga yordam beradi.</p>
    <label>Ism *<input className="input mt-1" required maxLength={100} value={firstName} onChange={(event) => setFirstName(event.target.value)} autoComplete="given-name" /></label>
    <label>Familiya<input className="input mt-1" maxLength={100} value={lastName} onChange={(event) => setLastName(event.target.value)} autoComplete="family-name" /></label>
    <label>Viloyat *<select className="input mt-1" required value={region} onChange={(event) => { setRegion(event.target.value); setDistrict(""); }}><option value="">Tanlang</option>{optionsWithCurrent(PROFILE_REGIONS, region).map((item) => <option key={item}>{item}</option>)}</select></label>
    <label>Tuman *<select className="input mt-1" required disabled={!region} value={district} onChange={(event) => setDistrict(event.target.value)}><option value="">Tanlang</option>{optionsWithCurrent(districtsOf(region), district).map((item) => <option key={item}>{item}</option>)}</select></label>
    {error && <p className="text-sm text-danger" role="alert">{error}</p>}
    <button className="btn btn-primary" disabled={busy}>{busy ? "Saqlanmoqda…" : "Saqlash va e'lon berish"}</button>
  </form>;
}
