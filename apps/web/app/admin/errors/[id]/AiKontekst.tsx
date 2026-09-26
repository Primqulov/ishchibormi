"use client";
/**
 * "3.12.3 · L — AI uchun tayyor kontekst" paneli.
 *
 * # NIMA UCHUN KERAK
 *
 * Xatolikni AI'ga tushuntirish uchun odam hozir oltita joydan qo'lda
 * nusxa oladi: stack trace, qurilma, so'rov, qadamlar, versiya, muhit.
 * Yo'lda yarmi tushib qoladi va AI noto'g'ri javob beradi. Bu panel
 * hammasini BIR tugmada, o'zgarmas tartibda beradi.
 *
 * # MATN QAYERDAN KELADI
 *
 * `GET /api/admin/errors/{id}/context` — matnni SERVER yig'adi. Bu ekran
 * uni faqat ko'rsatadi va nusxalaydi. Sabab oddiy: niqoblash matn
 * tug'ilgan joyda bo'lishi kerak, aks holda "niqoblangan" yozuvi
 * brauzerdagi va'da bo'lib qolardi. Ikkinchi sabab — AI tahlili aynan
 * SHU matnni ko'radi (`internal/admin/errai.go`), ya'ni ekranda ko'ringan
 * narsa modelga borgan narsa bilan bir xil.
 *
 * Server javob bermasa (tarmoq yoki xato), ekran shu HAQIQIY ma'lumotdan
 * brauzerda yig'ilgan zaxira matnga tushadi (`xatoKontekst.ts`)
 * va buni "namuna matn" nishoni bilan OCHIQ aytadi: admin haqiqiy
 * server matnini mahalliy zaxiradan ajrata olishi shart.
 *
 * # XAVFSIZLIK
 *
 * · "Shaxsiy ma'lumotlarni niqoblash" — o'chirilmaydigan katak. Bu
 *   bezak emas: niqoblash matn TUG'ILGAN joyda bo'ladi
 *   (`internal/admin/errexport.go · maskSecrets`), uni o'chiradigan
 *   parametr API'da umuman yozilmagan. Katak shuning uchun `disabled`.
 * · Kontekst — panelning eng zich diagnostika ma'lumoti. Serverda u
 *   AUDIT qilinadi va admin boshiga 5 daqiqada 20 marta bilan
 *   cheklanadi (`exportLimiter`). Shu sababli bu yerda ham "tez-tez
 *   bosish" qulayligi ataylab qo'shilmagan.
 * · Telegram'ga yuborish — TASHQARIGA chiqadigan amal, shuning uchun
 *   tasdiq oynasi va 60 soniyalik sovish oynasi bor (server ham
 *   `tgCooldown` bilan shuni talab qiladi).
 * · Matn tashqi AI xizmatiga tushadi. Niqob telefon, IP, token va OTP'ni
 *   olib tashlaydi, lekin endpoint nomlari va modul tuzilishi qoladi —
 *   panel buni ochiq aytadi, admin qaror qabul qilsin.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Download, Lock, Send, ShieldCheck } from "lucide-react";
import type {
  APIError,
  AdminErrorDetail,
  AdminErrorGroup,
  XatoKontekst,
  XatoKontekstKalit,
} from "@/lib/api";
import { api } from "@/lib/api";
import { AdminModal } from "@/components/admin/AdminModal";
import {
  HOSHIYA,
  HOSHIYA_OCH,
  HOSHIYA_QUYUQ,
  IK,
  KO_K,
  KUL,
  OCH_KUL,
  ORANJ,
  QIZIL,
  XIRA,
  XIRA_QUYUQ,
  tugma,
} from "@/components/admin/ui";
import { FOKUS, Izohcha, Karta, Nishon, nusxaOl } from "@/components/admin/xatoQismlar";
import { KONTEKST_YOQ, kontekstYasash } from "@/components/admin/xatoKontekst";
import { son } from "@/components/admin/xato";

type Format = "md" | "json" | "txt";

const FORMATLAR: { kod: Format; nomi: string }[] = [
  { kod: "md", nomi: "Markdown" },
  { kod: "json", nomi: "JSON" },
  { kod: "txt", nomi: "Matn" },
];

/** Figma L · "Nimalar qo'shilsin" ro'yxati, aynan shu tartibda. */
const BOLAKLAR: { kod: XatoKontekstKalit; nomi: string }[] = [
  { kod: "stack", nomi: "Stack trace va xato matni" },
  { kod: "device", nomi: "Qurilma va muhit" },
  { kod: "request", nomi: "So'rov va javob" },
  { kod: "steps", nomi: "Oxirgi qadamlar (breadcrumbs)" },
  { kod: "code", nomi: "Tegishli kod manzillari" },
  { kod: "serverlog", nomi: "Server log (oxirgi 20 qator)" },
  { kod: "similar", nomi: "O'xshash oldingi xatoliklar" },
];

/**
 * Standart tanlov — SERVERDAGI `incDefault` bilan bir xil beshta bo'lim.
 *
 * `XatolikBatafsil` uni boshlang'ich qiymat sifatida oladi: ro'yxat u
 * yerda yashaydi, chunki AI tahlili ham AYNAN shu tanlovni yuboradi.
 */
export const KONTEKST_SUKUT: XatoKontekstKalit[] = [
  "stack",
  "device",
  "request",
  "steps",
  "code",
];

/** So'rovni har belgilashda emas, admin to'xtaganda yuborish oralig'i. */
const KUTISH = 450;

const ONIQ = "#f5f7fd"; // Figma: ko'rish oynasining foni.

/* ── Ko'rish oynasidagi ranglar ───────────────────────────────────────
   Bu "sintaksis bo'yash" emas — matnni O'QILADIGAN qilish. Admin bir
   qarashda bo'lim sarlavhalarini (ko'k) va xato qatorlarini (qizil)
   ajratsin: shu ikkisi kontekstda eng ko'p qaraladigan joy. */
const XATO_NAQSH = /CRASH|_TypeError|panic|timeout|→ [45]\d\d|not a subtype|aborted|no reachable/i;

function qatorRangi(l: string, format: Format): { color: string; fontWeight?: number } {
  if (format === "json") {
    if (/^\s*"(head|subtitle|sections|title|lines)"/.test(l)) return { color: KO_K, fontWeight: 600 };
    return { color: KUL };
  }
  if (l.startsWith("# ")) return { color: IK, fontWeight: 700 };
  if (l.startsWith("## ")) return { color: KO_K, fontWeight: 600 };
  if (format === "txt" && l.length > 3 && l.length < 60 && l === l.toUpperCase() && /[A-Z]/.test(l)) {
    return { color: KO_K, fontWeight: 600 };
  }
  if (XATO_NAQSH.test(l)) return { color: QIZIL };
  return { color: KUL };
}

/** Fayl nomi TASHQI matndan tug'iladi — yopiq belgilar to'plamiga solamiz. */
function xavfsizNom(v: string): string {
  const t = v.replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^[._-]+/, "");
  return t.slice(0, 80) || "kontekst.txt";
}

export default function AiKontekst({
  d,
  xabar,
  tanlangan,
  setTanlangan,
  qoldi,
  tgBoshla,
  guruhYangila,
}: {
  d: AdminErrorDetail;
  xabar: (kor: "ok" | "xato", sarlavha: string, tavsif?: string) => void;
  /** Bo'limlar tanlovi YUQORIDA yashaydi — AI tahlili ham shuni yuboradi. */
  tanlangan: XatoKontekstKalit[];
  setTanlangan: (f: (oldin: XatoKontekstKalit[]) => XatoKontekstKalit[]) => void;
  /** Telegram sovish oynasi — sarlavhadagi tugma bilan BITTA (server ham bitta). */
  qoldi: number;
  tgBoshla: (vaqt: number) => void;
  guruhYangila: (g: AdminErrorGroup) => void;
}) {
  const [format, setFormat] = useState<Format>("md");
  const [tgOyna, setTgOyna] = useState(false);
  const [tgYuborilmoqda, setTgYuborilmoqda] = useState(false);

  /** Serverdan kelgan matn. `null` — hali kelmagan yoki kelmadi. */
  const [serverK, setServerK] = useState<XatoKontekst | null>(null);
  const [namuna, setNamuna] = useState(false);
  const [yangilanmoqda, setYangilanmoqda] = useState(false);

  // Yig'ilmaydigan bo'limlar so'rovga QO'SHILMAYDI: server ularni
  // bilmaydi emas — biladi, lekin bo'sh qaytaradi va admin "so'radim,
  // kelmadi" deb o'ylardi. Katak baribir xira va bosilmaydi.
  const inc = useMemo(
    () => tanlangan.filter((k) => !KONTEKST_YOQ.includes(k)),
    [tanlangan],
  );
  const kalit = `${format}|${inc.join(",")}`;

  /**
   * Format va bo'lim kombinatsiyalari keshi.
   *
   * Admin uch formatni ketma-ket bosib ko'radi va yana birinchisiga
   * qaytadi — bu 5 daqiqada 20 ta cheklovning yarmini bekorga yeb
   * qo'yardi (`exportLimiter`). Kesh guruh ichida yashaydi, chunki matn
   * `count` o'zgarmaguncha o'zgarmaydi.
   */
  const kesh = useRef(new Map<string, XatoKontekst>());
  const soravRaqami = useRef(0);

  useEffect(() => {
    kesh.current.clear();
  }, [d.group.id, d.group.count]);

  /** Server javob bermasa ko'rsatiladigan, mahalliy yig'ilgan matn. */
  const zaxira = useMemo(() => kontekstYasash(d, format, inc), [d, format, inc]);

  useEffect(() => {
    if (inc.length === 0) return; // pastdagi qoida buni oldini oladi
    const keshda = kesh.current.get(kalit);
    if (keshda) {
      setServerK(keshda);
      setNamuna(false);
      setYangilanmoqda(false);
      return;
    }

    let bekor = false;
    const men = ++soravRaqami.current;
    setYangilanmoqda(true);
    const t = window.setTimeout(() => {
      void (async () => {
        try {
          const res = await api.get<XatoKontekst>(
            `/api/admin/errors/${encodeURIComponent(d.group.id)}/context` +
              `?format=${format}&include=${encodeURIComponent(inc.join(","))}`,
            { auth: "admin" } as any,
          );
          if (bekor || men !== soravRaqami.current) return;
          kesh.current.set(kalit, res);
          setServerK(res);
          setNamuna(false);
        } catch (e) {
          if (bekor || men !== soravRaqami.current) return;
          const err = e as APIError | undefined;
          if (err?.code === "rate_limited") {
            xabar(
              "xato",
              "Kontekst juda tez-tez so'ralmoqda",
              "Har bir admin uchun 5 daqiqada 20 ta eksport. Bir oz kutib, formatni qayta tanlang.",
            );
          }
          // Boshqa xatolarda jim tushamiz: matn ekranda baribir bo'ladi,
          // faqat "namuna" deb belgilanadi.
          setServerK(null);
          setNamuna(true);
        } finally {
          if (!bekor && men === soravRaqami.current) setYangilanmoqda(false);
        }
      })();
    }, KUTISH);

    return () => {
      bekor = true;
      window.clearTimeout(t);
    };
  }, [d.group.id, format, inc, kalit, xabar]);

  const kontekst = serverK ?? zaxira;

  const almash = useCallback(
    (k: XatoKontekstKalit) => {
      setTanlangan((oldin) => {
        if (!oldin.includes(k)) return [...oldin, k];
        // Hammasini olib tashlab bo'lmaydi: bo'sh `include` ni server
        // "standart beshtasi" deb tushunadi (`parseInclude`), ya'ni
        // ekrandagi belgisiz kataklar yolg'on bo'lib qolardi.
        if (oldin.filter((x) => !KONTEKST_YOQ.includes(x)).length <= 1) {
          xabar(
            "xato",
            "Kamida bitta bo'lim qolishi kerak",
            "Bo'sh kontekstdan AI ham, Telegram ham foyda ko'rmaydi.",
          );
          return oldin;
        }
        return oldin.filter((x) => x !== k);
      });
    },
    [setTanlangan, xabar],
  );

  const nusxala = useCallback(async () => {
    const ok = await nusxaOl(kontekst.text);
    if (ok) {
      xabar(
        "ok",
        namuna ? "Mahalliy nusxa nusxalandi" : "AI uchun kontekst nusxalandi",
        namuna
          ? `${son(kontekst.chars)} belgi · server javob bermadi, matn brauzerda yig'ildi — sahifani yangilab qayta oling`
          : `${son(kontekst.chars)} belgi · ~${son(kontekst.tokens)} token · endi uni AI'ga tashlashingiz mumkin`,
      );
    } else {
      xabar(
        "xato",
        "Nusxalab bo'lmadi",
        "Brauzer bufer ruxsatini bermadi. Matnni oynadan qo'lda belgilab nusxalang.",
      );
    }
  }, [kontekst, namuna, xabar]);

  const yuklab = useCallback(() => {
    try {
      const tur = format === "json" ? "application/json" : "text/plain";
      const blob = new Blob([kontekst.text], { type: `${tur};charset=utf-8` });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = xavfsizNom(kontekst.filename);
      document.body.appendChild(a);
      a.click();
      a.remove();
      // Brauzer yuklashni boshlashi uchun ozgina vaqt beramiz, keyin
      // xotirani bo'shatamiz — aks holda blob sahifa yopilguncha qoladi.
      window.setTimeout(() => URL.revokeObjectURL(url), 2000);
      xabar("ok", "Fayl yuklab olindi", xavfsizNom(kontekst.filename));
    } catch {
      xabar("xato", "Faylni yaratib bo'lmadi", "Brauzer yuklab olishga ruxsat bermadi.");
    }
  }, [format, kontekst, xabar]);

  /**
   * Telegram — KONTEKST BILAN.
   *
   * Sarlavhadagi "Telegram'ga yuborish" tugmasi qisqa ogohlantirish
   * yuboradi, bu esa `context: true` bilan to'liq matnni qo'shadi. Ikkisi
   * bitta endpoint va bitta sovish oynasini bo'lishadi — server ham
   * shunday hisoblaydi (`PostErrorTelegram · tgCooldown`).
   */
  const tgYubor = useCallback(async () => {
    if (tgYuborilmoqda) return;
    setTgYuborilmoqda(true);
    try {
      const gr = await api.post<AdminErrorGroup>(
        `/api/admin/errors/${encodeURIComponent(d.group.id)}/telegram`,
        { context: true, include: inc },
        { auth: "admin" } as any,
      );
      if (gr?.id) guruhYangila(gr);
      setTgOyna(false);
      tgBoshla(Date.now());
      xabar(
        "ok",
        "Telegram'ga yuborildi",
        `${d.group.ref} · ogohlantirish kanaliga kontekst bilan tushdi.`,
      );
    } catch (e) {
      const err = e as APIError | undefined;
      const qolgan = Number((err?.details as { retryAfter?: number } | undefined)?.retryAfter);
      if (err?.code === "cooldown" && Number.isFinite(qolgan)) {
        // Sovish oynasini SERVER hisoblaydi — taymerni o'sha qiymatdan
        // boshlaymiz, o'zimizniki bilan taxmin qilmaymiz.
        setTgOyna(false);
        tgBoshla(Date.now() - (60_000 - qolgan * 1000));
        xabar("xato", "Hozir yuborilgan", `${qolgan} soniyadan keyin qayta urinib ko'ring.`);
      } else if (err?.code === "tg_not_configured") {
        setTgOyna(false);
        xabar(
          "xato",
          "Telegram sozlanmagan",
          "TELEGRAM_BOT_TOKEN va ERROR_ALERT_CHAT_ID berilmagan — kanalga yuborib bo'lmaydi.",
        );
      } else if (err?.code === "rate_limited") {
        xabar(
          "xato",
          "Kontekst juda tez-tez so'ralmoqda",
          "Kontekst bilan yuborish ham eksport hisoblanadi. Bir necha daqiqadan keyin urinib ko'ring.",
        );
      } else {
        xabar(
          "xato",
          "Telegram'ga yuborib bo'lmadi",
          err?.message || "Server javob bermadi. Qayta urinib ko'ring.",
        );
      }
    } finally {
      setTgYuborilmoqda(false);
    }
  }, [d.group.id, d.group.ref, guruhYangila, inc, tgBoshla, tgYuborilmoqda, xabar]);

  const qatorlar = useMemo(() => kontekst.text.split("\n"), [kontekst.text]);

  return (
    <>
      <Karta
        sarlavha="AI uchun tayyor kontekst"
        amal={
          <div className="flex shrink-0 items-center gap-[8px]">
            {/* Server matnini mahalliy nusxadan ajratib turadigan yagona
                belgi — shuning uchun u tugmaning YONIDA. */}
            {namuna && (
              <span title="Server javob bermadi. Matn shu sahifadagi ma'lumotdan brauzerda yig'ildi — bo'limlar tartibi va ba'zi qatorlar server nusxasidan farq qilishi mumkin.">
                <Nishon nomi="mahalliy nusxa" rang={ORANJ} nuqta />
              </span>
            )}
            {yangilanmoqda && (
              <span className="text-[11.5px] leading-4" style={{ color: XIRA_QUYUQ }}>
                yangilanmoqda…
              </span>
            )}
            <button
              type="button"
              onClick={nusxala}
              className={`${tugma("asosiy", { kichik: true }).className} ${FOKUS}`}
              style={{ ...tugma("asosiy", { kichik: true }).style, height: 30 }}
            >
              <Copy size={13} aria-hidden />
              Nusxa olish
            </button>
          </div>
        }
        tana="p-[18px]"
      >
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
          {/* ── Chap: format + ko'rish oynasi ─────────────────────── */}
          <div className="flex min-w-0 flex-col gap-[10px]">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div
                className="inline-flex h-[30px] items-center gap-[2px] rounded-[9px] p-[2px]"
                style={{ background: "#eef1fb" }}
                role="tablist"
                aria-label="Kontekst formati"
              >
                {FORMATLAR.map((f) => {
                  const faol = format === f.kod;
                  return (
                    <button
                      key={f.kod}
                      type="button"
                      role="tab"
                      aria-selected={faol}
                      onClick={() => setFormat(f.kod)}
                      className={`h-[26px] rounded-[7px] px-[11px] text-[12px] font-medium transition-colors ${FOKUS}`}
                      style={
                        faol
                          ? { background: "#fff", color: IK, boxShadow: "0 1px 2px rgba(11,28,48,0.10)" }
                          : { color: OCH_KUL }
                      }
                    >
                      {f.nomi}
                    </button>
                  );
                })}
              </div>
              <span className="text-[12px] leading-4" style={{ color: OCH_KUL }}>
                {son(kontekst.chars)} belgi · ~{son(kontekst.tokens)} token
              </span>
            </div>

            <div
              className="max-h-[420px] min-w-0 overflow-auto rounded-[10px] px-[14px] py-[12px]"
              style={{ background: ONIQ, boxShadow: `inset 0 0 0 1px ${HOSHIYA}` }}
              tabIndex={0}
              role="region"
              aria-label="Kontekst matni"
            >
              {/* `white-space: pre-wrap` — matn tartibi saqlanadi, lekin uzun
                  qator kartadan chiqib ketmaydi. */}
              <pre className="m-0 whitespace-pre-wrap break-words font-mono text-[12px] leading-[21px]">
                {qatorlar.map((l, i) => (
                  <span key={i} style={qatorRangi(l, format)}>
                    {l || " "}
                    {"\n"}
                  </span>
                ))}
              </pre>
            </div>

            <div className="flex items-center gap-[7px] text-[12px] leading-4" style={{ color: "#14663f" }}>
              <ShieldCheck size={14} aria-hidden />
              Telefon, IP, token va OTP kodlari niqoblangan
            </div>
          </div>

          {/* ── O'ng: nimalar qo'shilsin ──────────────────────────── */}
          <div className="flex min-w-0 flex-col gap-[14px]">
            <div>
              <div className="mb-[6px] text-[12px] font-semibold leading-4" style={{ color: IK }}>
                Nimalar qo'shilsin
              </div>
              <div className="flex flex-col">
                {BOLAKLAR.map((b) => {
                  const yoq = KONTEKST_YOQ.includes(b.kod);
                  const belgili = !yoq && tanlangan.includes(b.kod);
                  return (
                    <label
                      key={b.kod}
                      className={`flex min-h-[32px] items-center gap-[9px] ${yoq ? "cursor-not-allowed" : "cursor-pointer"}`}
                      title={yoq ? "Hozircha yig'ilmaydi — backend bu ma'lumotni saqlamaydi." : undefined}
                    >
                      <input
                        type="checkbox"
                        className="peer sr-only"
                        checked={belgili}
                        disabled={yoq}
                        onChange={() => almash(b.kod)}
                      />
                      <span
                        aria-hidden
                        className="grid h-[16px] w-[16px] shrink-0 place-items-center rounded-[5px] peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-1 peer-focus-visible:outline-[#004ac6]"
                        style={
                          belgili
                            ? { background: KO_K }
                            : {
                                background: yoq ? HOSHIYA_OCH : "#fff",
                                boxShadow: `inset 0 0 0 1px ${yoq ? HOSHIYA_OCH : HOSHIYA_QUYUQ}`,
                              }
                        }
                      >
                        {belgili && <Check size={11} color="#fff" strokeWidth={3} aria-hidden />}
                      </span>
                      <span
                        className="min-w-0 text-[12.5px] leading-[17px]"
                        style={{ color: yoq ? XIRA : KUL }}
                      >
                        {b.nomi}
                      </span>
                    </label>
                  );
                })}

                {/* O'chirilmaydigan katak. `disabled` — bu tanlov emas. */}
                <label
                  className="flex min-h-[32px] cursor-not-allowed items-center gap-[9px]"
                  title="Niqoblash serverda, matn tug'ilgan joyda bajariladi — uni o'chiradigan parametr API'da yo'q."
                >
                  <input type="checkbox" className="sr-only" checked disabled readOnly />
                  <span
                    aria-hidden
                    className="grid h-[16px] w-[16px] shrink-0 place-items-center rounded-[5px]"
                    style={{ background: "#1fa463" }}
                  >
                    <Check size={11} color="#fff" strokeWidth={3} aria-hidden />
                  </span>
                  <span className="flex min-w-0 items-center gap-[6px] text-[12.5px] leading-[17px]" style={{ color: "#14663f" }}>
                    Shaxsiy ma'lumotlarni niqoblash
                    <Lock size={11} aria-hidden />
                  </span>
                </label>
              </div>
            </div>

            <Izohcha kor="yashil" ikon={<ShieldCheck size={14} aria-hidden />}>
              «Shaxsiy ma'lumotlarni niqoblash» o'chirilmaydi. Telefon, IP manzil, token va OTP
              kodlari kontekstga hech qachon tushmaydi. Endpoint nomlari va modul tuzilishi esa
              qoladi — matnni faqat ishonchli AI xizmatiga bering.
            </Izohcha>

            <div>
              <div className="mb-[8px] text-[12px] font-semibold leading-4" style={{ color: IK }}>
                Boshqa amallar
              </div>
              <div className="flex flex-col gap-[8px]">
                <button
                  type="button"
                  onClick={yuklab}
                  className={`${tugma("ikkilamchi").className} ${FOKUS} w-full`}
                  style={tugma("ikkilamchi").style}
                >
                  <Download size={15} aria-hidden />
                  Fayl sifatida yuklab olish (.{format})
                </button>
                <button
                  type="button"
                  onClick={() => setTgOyna(true)}
                  disabled={qoldi > 0}
                  className={`${tugma("ikkilamchi", { ochiq: qoldi > 0 }).className} ${FOKUS} w-full`}
                  style={tugma("ikkilamchi", { ochiq: qoldi > 0 }).style}
                >
                  <Send size={15} aria-hidden />
                  {qoldi > 0 ? `Telegram — ${qoldi} s kuting` : "Telegram'ga yuborish"}
                </button>
              </div>
              {qoldi > 0 && (
                <p className="mt-[6px] text-[11.5px] leading-4" style={{ color: XIRA_QUYUQ }}>
                  Ketma-ket yuborishning oldini olish uchun 60 soniyalik oraliq.
                </p>
              )}
            </div>
          </div>
        </div>
      </Karta>

      {/* ── Tasdiq: Telegram tashqariga chiqadigan amal ─────────────── */}
      <AdminModal
        open={tgOyna}
        onClose={() => {
          if (!tgYuborilmoqda) setTgOyna(false);
        }}
        title="Telegram'ga yuborilsinmi?"
        maxWidth="max-w-[460px]"
        footer={
          <>
            <button
              type="button"
              onClick={() => setTgOyna(false)}
              disabled={tgYuborilmoqda}
              className={tugma("ikkilamchi", { ochiq: tgYuborilmoqda }).className}
              style={tugma("ikkilamchi", { ochiq: tgYuborilmoqda }).style}
            >
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={tgYubor}
              disabled={tgYuborilmoqda}
              className={tugma("asosiy", { ochiq: tgYuborilmoqda }).className}
              style={tugma("asosiy", { ochiq: tgYuborilmoqda }).style}
            >
              <Send size={15} aria-hidden />
              {tgYuborilmoqda ? "Yuborilmoqda…" : "Yuborish"}
            </button>
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <p className="text-[13px] leading-[19px]" style={{ color: KUL }}>
            <span className="font-semibold" style={{ color: IK }}>
              {d.group.ref}
            </span>{" "}
            konteksti <span className="font-semibold">#dev-alerts</span> kanaliga yuboriladi —{" "}
            {son(kontekst.chars)} belgi, {kontekst.include.length} ta bo'lim.
          </p>
          {/* Telegram xabari 4096 belgi bilan chegaralangan, shuning uchun
              server matnni 3400 belgida kesadi va kesilganini o'zi yozib
              qo'yadi. Buni oldindan aytmasak, kanalda yarim stack trace
              ko'rinib, "yuborilmadi" degan taassurot qolardi. */}
          {kontekst.chars > 3400 && (
            <Izohcha kor="kok">
              Matn uzun — kanalga birinchi 3400 belgisi tushadi, oxirida «qisqartirildi» belgisi
              bilan. To'lig'i shu sahifada qoladi.
            </Izohcha>
          )}
          <Izohcha kor="sariq">
            Bu ma'lumot paneldan TASHQARIGA chiqadi va kanaldagi hamma uni ko'radi. Telefon va IP
            niqoblangan, lekin stack trace, endpoint nomlari va modul tuzilishi matnda qoladi.
          </Izohcha>
        </div>
      </AdminModal>
    </>
  );
}
