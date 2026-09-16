"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Check, ExternalLink } from "lucide-react";
import { api, APIError, setAccess, User } from "@/lib/api";
import { AUTH_BOT_USERNAME } from "@/lib/contact";
import { Button } from "@/components/ui/Button";
import { Logo } from "@/components/Logo";
import { T, useT } from "@/components/T";
import { LangMenu } from "@/components/LangMenu";
import { ThemeToggle } from "@/components/ThemeToggle";
import { TelegramIcon } from "@/components/icons/TelegramIcon";

type Req = { tgToken: string; devCode?: string };
type Verify = { accessToken: string; refreshToken: string; user: User };

/** Figma "01 · Ro'yxatdan o'tish (Kod kiritish)": chapda ko'k panel, o'ngda kod kartasi. */
export default function LoginPage() {
  const router = useRouter();
  const t = useT();
  const [tgToken, setTgToken] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  // Ro'yxatdan o'tish nazarda tutilgan rozilik bilan davom etmaydi: shartlar va
  // maxfiylik siyosatiga rozilik aniq belgilanishi shart.
  const [agreed, setAgreed] = useState(false);

  useEffect(() => {
    api.post<Req>("/api/auth/otp/request", {}).then((r) => {
      setTgToken(r.tgToken);
    }).catch(() => {
      setError("Serverga ulanib bo'lmadi. Birozdan so'ng qayta urinib ko'ring.");
    });
    // Faqat bir marta ishlaydi (mount'da). `t` bilan bog'lamaymiz — u har
    // renderda yangi funksiya, aks holda OTP so'rovi takrorlanib ketadi.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Login va saytdagi boshqa bot havolalari bitta sozlamadan olinadi.
  // Backend eski botUrl qaytarsa ham OTP sessiyasi yangi botga uzatiladi.
  const effectiveBotUrl =
    tgToken ? `https://t.me/${AUTH_BOT_USERNAME}?start=${encodeURIComponent(tgToken)}` : "";
  // t.me ba'zi tarmoqlarda brauzerda ochilmaydi (DNS bloklanishi mumkin), lekin
  // Telegram ilovasi ishlaydi. tg:// havolasi DNS'siz to'g'ridan-to'g'ri
  // o'rnatilgan Telegram ilovasini ochadi — "This site can't be reached" holatida zaxira yo'l.
  const tgAppUrl =
    tgToken ? `tg://resolve?domain=${AUTH_BOT_USERNAME}&start=${encodeURIComponent(tgToken)}` : "";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (code.length < 6) return;
    // Tugma allaqachon o'chirilgan bo'ladi — bu Enter bilan yuborish yo'lini
    // ham yopadi.
    if (!agreed) {
      setError(t("Davom etish uchun foydalanish shartlari va maxfiylik siyosatiga rozilik bildiring."));
      return;
    }
    setSubmitting(true);
    try {
      const v = await api.post<Verify>("/api/auth/otp/verify", { token: tgToken, code });
      // Faqat access token saqlanadi. Refresh token localStorage'da saqlanmaydi
      // (web ilova refresh oqimini ishlatmaydi) — XSS hujum yuzasini kamaytiradi.
      setAccess(v.accessToken);
      router.replace(v.user.onboardingCompleted ? "/dashboard" : "/onboarding");
    } catch (err: unknown) {
      // Backend xabari texnik yoki inglizcha bo'lishi mumkin. Uni ekranga
      // bevosita chiqarmaymiz; faqat xato kodiga mos, foydalanuvchi uchun
      // tushunarli mahalliy xabarni ko'rsatamiz.
      const errorCode =
        typeof err === "object" && err !== null && "code" in err
          ? String(err.code)
          : "";

      switch (errorCode) {
        case "invalid_code":
          setError(t("Kod noto'g'ri yoki uning muddati tugagan. Yangi kod olib, qayta urinib ko'ring."));
          break;
        case "no_phone_bound":
          setError(t("Avval Telegram botga telefon raqamingizni yuboring, keyin kodni kiriting."));
          break;
        case "account_blocked":
          setError(t("Hisobingiz bloklangan. Yordam xizmatiga murojaat qiling."));
          break;
        // Avtomatik moderatsiya bloki. Backend `details.bannedUntil` ni ham
        // qaytaradi — sanani ko'rsatsak foydalanuvchi qachon qaytishini
        // biladi va yordam xizmatiga bekorga murojaat qilmaydi.
        case "account_banned": {
          const until = (err as APIError)?.details?.bannedUntil;
          const date = until ? new Date(String(until)).toLocaleDateString("uz-UZ") : "";
          setError(
            date
              ? t("Hisobingiz qoidabuzarlik sababli bloklangan. Blok muddati:") + " " + date
              : t("Hisobingiz qoidabuzarlik sababli bloklangan. Yordam xizmatiga murojaat qiling.")
          );
          break;
        }
        case "rate_limited":
          setError(t("Juda ko'p urinish bo'ldi. Biroz kutib, qayta urinib ko'ring."));
          break;
        case "offline":
          setError(t("Internet aloqasi yo'q. Tarmoqni tekshirib, qayta urinib ko'ring."));
          break;
        case "server_unavailable":
        case "server_error":
          setError(t("Server vaqtincha ishlamayapti. Internetingiz ishlayapti — birozdan so'ng qayta urinib ko'ring."));
          break;
        default:
          setError(t("Kodni tekshirib bo'lmadi. Birozdan so'ng qayta urinib ko'ring."));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen grid lg:grid-cols-[38%_1fr]">
      {/* ── Chap panel — Figma: ko'k gradient ─────────────────────── */}
      <aside className="gradient-hero text-white hidden lg:flex flex-col justify-between p-12">
        <Link href="/" className="text-[21px] font-black tracking-[-0.3px]">
          <span className="text-white">Ishchi</span><span style={{ color: "var(--accent)" }}>Bormi</span>
        </Link>
        <div>
          <h2 className="text-[38px] font-black leading-[1.15] tracking-[-1.2px]">
            <T>Ish topish va</T><br /><T>ishchi yollash —</T><br /><T>bir joyda.</T>
          </h2>
          <p className="mt-5 text-[14px] text-white/75 leading-relaxed max-w-sm">
            <T>Bitta hisob — ham ish qidiring, ham ishchi yollang. Telegram bot orqali bir daqiqada ro'yxatdan o'ting.</T>
          </p>
          <div className="mt-9 grid grid-cols-3 gap-4 max-w-md">
            {[["30 soniya", "Ro'yxatdan o'tish"], ["0 so'm", "Ilovadan foydalanish haqi"], ["24/7", "Qo'llab-quvvatlash"]].map(([v, l]) => (
              <div key={l}>
                <div className="text-[20px] font-black"><T>{v}</T></div>
                <div className="text-[11.5px] text-white/70 mt-0.5"><T>{l}</T></div>
              </div>
            ))}
          </div>
        </div>
      </aside>

      {/* ── O'ng panel — kod kartasi ─────────────────────────────── */}
      <div className="flex flex-col" style={{ background: "var(--bg-subtle)" }}>
        <header className="flex items-center justify-between px-5 sm:px-10 py-5">
          <div className="lg:hidden"><Logo /></div>
          <div className="flex items-center gap-2 ml-auto">
            <LangMenu />
            <ThemeToggle />
          </div>
        </header>

        <main className="flex-1 grid place-items-center px-5 sm:px-10 pb-10">
          <form onSubmit={submit} className="card w-full max-w-[480px] p-7 sm:p-8 animate-slide-up">
            <h1 className="text-[24px] font-black heading tracking-[-0.6px]"><T>Ro'yxatdan o'tish</T></h1>
            <p className="mt-1.5 text-[13.5px] muted">
              <T>Telegram botimiz orqali kod oling va quyida kiriting.</T>
            </p>

            {/* 1-qadam */}
            <div className="mt-6 rounded-xl p-4 flex gap-3" style={{ background: "var(--brand-soft)" }}>
              <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl text-white"
                    style={{ background: "var(--brand)" }}>
                <TelegramIcon className="h-4 w-4" />
              </span>
              <div>
                <div className="text-[13px] font-bold" style={{ color: "var(--brand)" }}>
                  <T>1-qadam · Telegram bot</T>
                </div>
                <div className="mt-1 text-[12.5px] muted leading-relaxed">
                  <T>Botga o'ting, «Start» tugmasini bosing va 6 xonali kodni oling.</T>
                </div>
              </div>
            </div>

            <a
              href={effectiveBotUrl || "#"}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => { if (!effectiveBotUrl) e.preventDefault(); }}
              className="btn btn-outline w-full mt-3 gap-2"
            >
              <T>Telegram botni ochish</T><ExternalLink size={14} />
            </a>

            {/* t.me brauzerda ochilmasa (DNS bloklangan bo'lsa) — ilovada ochish */}
            {tgAppUrl && (
              <a href={tgAppUrl} className="mt-2 block text-center text-[12px] subtle underline underline-offset-2 hover:text-[color:var(--text)]">
                <T>Havola ochilmayaptimi? Telegram ilovasida ochish</T>
              </a>
            )}

            {/* 2-qadam — kod kataklari */}
            <div className="mt-6">
              <div className="text-[13px] font-bold heading mb-2.5"><T>2-qadam · Kodni kiriting</T></div>
              <OtpBoxes value={code} onChange={(v) => { setCode(v); setError(""); }} />
              {error && (
                <div className="mt-3 text-[13px] text-danger flex items-center gap-1.5 animate-fade-in">
                  <span className="h-1.5 w-1.5 rounded-full bg-danger shrink-0" />{error}
                </div>
              )}
            </div>

            {/* Rozilik — tugmadan yuqorida turadi, shunda tugma nega
                o'chirilganini foydalanuvchi darhol ko'radi. Hujjatlar yangi
                oynada ochiladi: sahifadan chiqib ketilsa, kiritilgan kod va
                OTP sessiyasi yo'qoladi. */}
            <ConsentCheck checked={agreed} onChange={(v) => { setAgreed(v); setError(""); }} />

            <Button type="submit" size="lg" fullWidth className="mt-5" disabled={code.length < 6 || !agreed} loading={submitting}>
              {submitting ? <T>Tekshirilmoqda…</T> : <T>Davom etish</T>}
            </Button>
          </form>
        </main>

        <footer className="px-5 sm:px-10 py-5 flex flex-col sm:flex-row justify-between gap-2 text-[12.5px] subtle">
          <div>© 2026 Ishchi Bormi</div>
          <div className="flex gap-5">
            <Link href="/yordam" className="hover:text-[color:var(--text)]"><T>Yordam</T></Link>
            <Link href="/maxfiylik-siyosati" className="hover:text-[color:var(--text)]"><T>Maxfiylik siyosati</T></Link>
          </div>
        </footer>
      </div>
    </div>
  );
}

/** Foydalanish shartlari va Maxfiylik siyosatiga aniq rozilik.
 *
 *  Haqiqiy `<input type="checkbox">` — klaviatura va skrin-riderlar uchun;
 *  ko'rinadigan katak esa `peer-*` sinflari bilan chiziladi. Matn `<label>`
 *  ichida emas: aks holda havolani bosish katakni ham almashtirib yuborardi. */
function ConsentCheck({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  const t = useT();
  // Ko'rinadigan matn `<label>` tashqarisida bo'lgani uchun katakning nomi
  // aria-label bilan beriladi — aks holda skrin-rider nomsiz katakni o'qiydi.
  const label = t("Men Foydalanish shartlari va Maxfiylik siyosati bilan tanishdim va roziman.");

  return (
    <div className="mt-5 flex items-start gap-2.5">
      <input
        id="consent"
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        aria-label={label}
        className="peer sr-only"
      />
      {/* Katak 20px, `after:-inset-2` esa bosish maydonini ~36px'ga kengaytiradi
          (layout o'lchamiga ta'sir qilmaydi). Fokus halqasi `peer-*` orqali —
          label input'ning bevosita qo'shnisi. */}
      <label
        htmlFor="consent"
        className="relative mt-px grid h-5 w-5 shrink-0 cursor-pointer place-items-center rounded-sm
                   border-[1.5px] transition-colors after:absolute after:-inset-2 after:content-['']
                   peer-focus-visible:ring-[3px] peer-focus-visible:ring-[color:var(--ring)]"
        style={{
          borderColor: checked ? "var(--brand)" : "var(--border-strong)",
          background: checked ? "var(--brand)" : "var(--card)",
        }}
      >
        <Check
          size={13}
          strokeWidth={3.5}
          className="text-white transition-opacity"
          style={{ opacity: checked ? 1 : 0 }}
        />
      </label>
      <p className="text-[12px] leading-relaxed muted">
        <T>Men</T>{" "}
        <Link
          href="/foydalanish-shartlari"
          target="_blank"
          rel="noreferrer"
          className="font-semibold underline underline-offset-2"
          style={{ color: "var(--brand)" }}
        >
          <T>Foydalanish shartlari</T>
        </Link>{" "}
        <T>va</T>{" "}
        <Link
          href="/maxfiylik-siyosati"
          target="_blank"
          rel="noreferrer"
          className="font-semibold underline underline-offset-2"
          style={{ color: "var(--brand)" }}
        >
          <T>Maxfiylik siyosati</T>
        </Link>{" "}
        <T>bilan tanishdim va roziman.</T>
      </p>
    </div>
  );
}

/** Figma: 6 ta alohida katak. Bitta yashirin input emas — har katak alohida,
 *  lekin qiymat yagona `code` satrida saqlanadi (qo'yib yuborish ham ishlaydi). */
function OtpBoxes({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const refs = useRef<(HTMLInputElement | null)[]>([]);

  function setAt(i: number, ch: string) {
    const digits = value.padEnd(6, " ").split("");
    digits[i] = ch || " ";
    onChange(digits.join("").replace(/\s+$/, "").replace(/\s/g, ""));
  }

  return (
    <div className="flex gap-2.5">
      {Array.from({ length: 6 }).map((_, i) => (
        <input
          key={i}
          ref={(el) => { refs.current[i] = el; }}
          inputMode="numeric"
          maxLength={1}
          value={value[i] || ""}
          onChange={(e) => {
            const raw = e.target.value.replace(/\D/g, "");
            if (raw.length > 1) {
              // Kodni to'liq qo'yib yuborish
              onChange(raw.slice(0, 6));
              refs.current[Math.min(5, raw.length - 1)]?.focus();
              return;
            }
            setAt(i, raw);
            if (raw && i < 5) refs.current[i + 1]?.focus();
          }}
          onKeyDown={(e) => {
            if (e.key === "Backspace" && !value[i] && i > 0) refs.current[i - 1]?.focus();
          }}
          onPaste={(e) => {
            const raw = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, 6);
            if (raw) { e.preventDefault(); onChange(raw); refs.current[Math.min(5, raw.length - 1)]?.focus(); }
          }}
          className="input !p-0 h-[52px] w-full text-center text-[22px] font-bold"
          aria-label={`${i + 1}-raqam`}
        />
      ))}
    </div>
  );
}
