"use client";
/**
 * "3.12.1 · Xatolik — batafsil" ekranining O'NTA kartasi.
 *
 * # NEGA ALOHIDA FAYL
 *
 * `XatolikBatafsil.tsx` da holat, so'rov, oynalar va ruxsat tekshiruvi
 * yashaydi — u yerga kartalarni ham qo'shsak, bitta qatorni tuzatish
 * uchun 1500 qatorli faylni qayta o'qishga to'g'ri kelardi
 * (`components/admin/xatoQismlar.tsx` bosh izohidagi bir xil sabab).
 * Bu yerdagi komponentlar FAQAT ko'rsatadi: hech biri so'rov yubormaydi,
 * hech biri `useState` da ma'lumot saqlamaydi — kengaytirish tugmasidan
 * boshqa (u ekranning o'zi bilan bog'liq emas).
 *
 * # O'LCHAMLAR QAYERDAN
 *
 * Figma 394:2 dan aynan: karta sarlavhasi 52, jadval sarlavhasi 40,
 * jadval qatori 44, foydalanuvchi qatori 48, diagramma balandligi 148,
 * ustun eni 22 va radiusi 4. Hoshiyalar `inset` soya bilan chiziladi:
 * Figma'da chegara STROKE INSIDE turadi va qutini kengaytirmaydi.
 */
import { useState } from "react";
import type { ReactNode } from "react";
import {
  Apple,
  CircleSlash,
  CornerDownRight,
  Footprints,
  Globe,
  Lock,
  RotateCcw,
  Server,
  Smartphone,
} from "lucide-react";
import type {
  AdminErrorDetail,
  AdminErrorGroup,
  XatoHolat,
  XatoQadam,
  XatoQurilma,
  XatoUlush,
} from "@/lib/api";
import {
  AVATAR_FON,
  HOSHIYA,
  HOSHIYA_QUYUQ,
  IK,
  KO_K,
  KUL,
  OCH_KUL,
  ORANJ,
  QIZIL,
  QUTI_FON,
  SARLAVHA_FON,
  SIYOH,
  XIRA_QUYUQ,
  YASHIL,
} from "@/components/admin/ui";
import {
  AVATAR_KO_K,
  Avatar,
  FOKUS,
  Izohcha,
  KQator,
  Karta,
  KartaAmal,
  Nishon,
} from "@/components/admin/xatoQismlar";
import { qurilmaYorliq } from "@/components/admin/xatoKontekst";
import {
  HOLAT,
  HOLAT_TARTIBI,
  NOMALUM,
  TEGISHSIZ,
  YOQ,
  kunNuqta,
  kunSoat,
  soat,
  soatSek,
  son,
} from "@/components/admin/xato";

/** Jadvalning zebra yo'lagi — ro'yxat sahifasidagi bilan bir xil. */
const ZEBRA = "#f8f9ff";

/* ── Kichik yordamchilar ───────────────────────────────────────────── */

/** Karta ichidagi "ma'lumot yo'q" yozuvi. Bo'sh karta buzuq ko'rinadi. */
function Bosh({ children }: { children: ReactNode }) {
  return (
    <p className="py-[14px] text-center text-[12.5px] leading-[17px]" style={{ color: XIRA_QUYUQ }}>
      {children}
    </p>
  );
}

/** Bo'sh bo'laklarni tashlab qo'shadi: "Android " kabi dumaloq qolmaydi. */
function birik(sep: string, ...q: (string | undefined)[]): string {
  return q.map((x) => (x ?? "").trim()).filter(Boolean).join(sep);
}

/**
 * "Bugun, 14:25" — soniyasiz ko'rinish (Figma: amallar tarixi ustuni).
 *
 * `kunSoat` soniya bilan qaytaradi (u hodisa vaqti uchun, u yerda soniya
 * MUHIM). Tarixda esa soniya shovqin, shuning uchun oxirgi ikki belgi
 * kesiladi — ikkinchi formatlovchi yozmaymiz, ikkisi bir-biridan
 * ajralib ketmasin.
 */
function vaqtQisqa(d: Date, hozir: number): string {
  return kunSoat(d, hozir).replace(/:\d\d$/, "");
}

/**
 * Javob kodining rangi: 5xx — qizil (server yiqildi), 4xx va "timeout" —
 * to'q sariq (so'rov yetib bormadi yoki rad etildi).
 */
function javobRang(v?: string): string {
  if (!v) return XIRA_QUYUQ;
  const n = Number(v);
  if (Number.isNaN(n)) return ORANJ; // "timeout", "aborted", "offline"
  if (n >= 500) return QIZIL;
  if (n >= 400) return ORANJ;
  return KUL;
}

/**
 * Stack trace'ning "begona" qatorlari — Dart/Flutter va Go standart
 * kutubxonasi. Ular xira bo'yaladi: tuzatiladigan joy DOIM o'z kodimizda,
 * `dart:async/zone.dart` da emas. Bu Figma'dagi ikki xil rangning
 * (#434655 va #9aa0b0) qoidasi.
 */
const BEGONA = /(?:^|[\s·])(?:dart:|package:flutter\b|net\/http|runtime[./]|internal\/poll|reflect\.|node_modules)/;

function oziniki(qator: string): boolean {
  return !BEGONA.test(qator);
}

/**
 * Xatolik turi nishoni (Figma: `_TypeError`).
 *
 * Serverda alohida "type" maydoni yo'q, shuning uchun uch pog'onali
 * taxmin: oxirgi qadam (`crash`) matni → xabardagi `…Error`/`…Exception`
 * so'zi → guruh kodi. Uchinchi pog'ona har doim mavjud, shuning uchun
 * nishon hech qachon bo'sh chiqmaydi.
 */
export function xatoTuri(d: AdminErrorDetail): string {
  const crash = (d.sample?.steps ?? []).find((q) => q.kind === "crash");
  if (crash) {
    const t = crash.text.split("—")[0].trim();
    if (t) return t;
  }
  const m = /\b([A-Za-z_][\w.]*(?:Error|Exception))\b/.exec(d.sample?.message ?? d.group.message ?? "");
  if (m) return m[1];
  return d.group.code;
}

/* ══ 1 · Stack trace · so'nggi hodisa ═══════════════════════════════ */

export function StackKarta({
  d,
  hozir,
  nusxala,
}: {
  d: AdminErrorDetail;
  hozir: number;
  nusxala: () => void;
}) {
  const s = d.sample;
  const qurilma = s ? birik(" ", s.device.os, s.device.osVersion) || s.device.platform : "";
  const versiya = s?.device.appVersion
    ? `${s.device.appVersion}${s.device.build ? ` (${s.device.build})` : ""}`
    : d.group.lastAppVersion;

  return (
    <Karta
      sarlavha="Stack trace · so'nggi hodisa"
      amal={
        <KartaAmal
          nomi="Nusxa olish"
          bosildi={s ? nusxala : undefined}
          ipucha={s ? "Xato matni va stack trace'ni buferga oladi" : "Nusxalash uchun namuna yo'q"}
        />
      }
      tana="p-[18px]"
    >
      {!s ? (
        <Bosh>
          Bu guruhda to'liq namuna saqlanmagan — stack trace faqat birinchi 20 ta hodisa uchun
          yoziladi.
        </Bosh>
      ) : (
        <div className="flex min-w-0 flex-col gap-[10px]">
          <div className="flex flex-wrap items-center gap-[8px]">
            <Nishon nomi={xatoTuri(d)} rang={QIZIL} />
            <span className="text-[12px] leading-4" style={{ color: OCH_KUL }}>
              {birik(
                " · ",
                versiya ? `Ilova versiyasi ${versiya}` : undefined,
                qurilma || undefined,
                kunSoat(new Date(s.at), hozir),
              )}
            </span>
          </div>

          {/* `pre` + `pre-wrap`: qator tartibi saqlanadi, lekin uzun yo'l
              kartadan chiqib ketmaydi (Figma'da blok 728 px). */}
          <div
            className="min-w-0 rounded-[10px] px-[14px] py-[12px]"
            style={{ background: AVATAR_FON }}
          >
            <pre className="m-0 whitespace-pre-wrap break-words text-[12px] leading-[21px]">
              {s.message && (
                <span className="font-semibold" style={{ color: QIZIL }}>
                  {s.message}
                  {"\n"}
                </span>
              )}
              {(s.stack ?? []).map((l, i) => (
                <span key={i} style={{ color: oziniki(l) ? KUL : XIRA_QUYUQ }}>
                  {l}
                  {"\n"}
                </span>
              ))}
              {!s.message && (s.stack ?? []).length === 0 && (
                <span style={{ color: XIRA_QUYUQ }}>Stack trace yozilmagan</span>
              )}
            </pre>
          </div>
        </div>
      )}
    </Karta>
  );
}

/* ══ 2 · Hodisalar chizig'i · oxirgi 24 soat ════════════════════════ */

/**
 * Ustun rangi — cho'qqiga NISBATAN.
 *
 * Figma'dagi uchta rang mutlaq songa bog'lanmagan: 8 ta hodisa ko'k, 9
 * tasi qizil, chunki cho'qqi 14 ta edi. Mutlaq chegara qo'ysak, kuniga
 * 3 hodisa keladigan guruhda hamma ustun kulrang bo'lib, grafik
 * o'qilmasdi. Chegaralar: 60 % — qizil, 25 % — ko'k.
 */
function ustunRang(n: number, cho: number): string {
  if (n <= 0 || cho <= 0) return HOSHIYA_QUYUQ;
  const u = n / cho;
  if (u >= 0.6) return QIZIL;
  if (u >= 0.25) return KO_K;
  return HOSHIYA_QUYUQ;
}

export function ChiziqKarta({ d }: { d: AdminErrorDetail }) {
  const cho = d.peak?.n ?? 0;
  const ustunlar = d.hourly ?? [];
  const choSana = d.peak?.at ? new Date(d.peak.at) : null;
  const oq = choSana && !Number.isNaN(choSana.getTime());

  // Vaqt o'qi — MA'LUMOTDAN, statik yozuvdan emas: 24 soatlik oyna
  // "hozir" bilan siljiydi, statik "00:00" esa yolg'on bo'lib qolardi.
  const belgilar = [0, 6, 12, 18, ustunlar.length - 1]
    .filter((i, k, r) => i >= 0 && i < ustunlar.length && r.indexOf(i) === k)
    .map((i) => soat(new Date(ustunlar[i].at)));

  return (
    <Karta
      sarlavha="Hodisalar chizig'i · oxirgi 24 soat"
      amal={
        <span className="shrink-0 text-[12px] font-medium leading-4" style={{ color: KO_K }}>
          {cho > 0 && oq ? `Eng yuqori: ${soat(choSana!)} — ${son(cho)} ta` : "Cho'qqi yo'q"}
        </span>
      }
      tana="p-[18px]"
    >
      {ustunlar.length === 0 ? (
        <Bosh>Oxirgi 24 soatda hodisa qayd etilmagan.</Bosh>
      ) : (
        <div className="flex min-w-0 flex-col gap-[10px]">
          <div className="flex h-[148px] items-end justify-between">
            {ustunlar.map((u, i) => {
              const s = new Date(u.at);
              const yaroqli = !Number.isNaN(s.getTime());
              return (
                <div
                  key={i}
                  className="w-[22px] shrink-0 rounded-[4px]"
                  style={{
                    height: cho > 0 ? Math.max(4, Math.round((u.n / cho) * 148)) : 4,
                    background: ustunRang(u.n, cho),
                  }}
                  title={`${yaroqli ? soat(s) : "?"} · ${son(u.n)} ta`}
                />
              );
            })}
          </div>
          <div
            className="flex h-[16px] items-start justify-between text-[11px] leading-[15px]"
            style={{ color: XIRA_QUYUQ }}
          >
            {belgilar.map((b, i) => (
              <span key={i}>{b}</span>
            ))}
          </div>
        </div>
      )}
    </Karta>
  );
}

/* ══ 3 · So'nggi hodisalar ══════════════════════════════════════════ */

const KORINADI = 5; // Figma: beshta qator.

export function HodisalarKarta({ d, hozir }: { d: AdminErrorDetail; hozir: number }) {
  const [hammasi, setHammasi] = useState(false);
  const qatorlar = hammasi ? d.recent : d.recent.slice(0, KORINADI);
  const yana = d.recent.length - KORINADI;

  return (
    <Karta
      sarlavha="So'nggi hodisalar"
      amal={
        yana > 0 ? (
          <KartaAmal
            nomi={hammasi ? "Kamroq ko'rsatish" : `Barchasini ko'rish (${son(d.recent.length)})`}
            bosildi={() => setHammasi((v) => !v)}
          />
        ) : undefined
      }
      tana=""
    >
      <div className="min-w-0 overflow-x-auto">
        <table className="w-full table-fixed border-collapse text-left">
          <colgroup>
            <col className="w-[132px]" />
            <col />
            <col className="w-[118px]" />
            <col className="w-[92px]" />
            <col className="w-[88px]" />
            <col className="w-[70px]" />
          </colgroup>
          <thead>
            <tr className="h-[40px]" style={{ background: SARLAVHA_FON }}>
              {["Vaqt", "Foydalanuvchi", "Platforma", "Ilova versiyasi", "Tarmoq"].map((t, i) => (
                <th
                  key={t}
                  className={`px-[6px] text-[11.5px] font-semibold leading-4 ${i === 0 ? "pl-[18px]" : ""}`}
                  style={{ color: IK }}
                >
                  {t}
                </th>
              ))}
              <th
                className="px-[6px] pr-[18px] text-right text-[11.5px] font-semibold leading-4"
                style={{ color: IK }}
              >
                Javob
              </th>
            </tr>
          </thead>
          <tbody>
            {qatorlar.length === 0 && (
              <tr>
                <td colSpan={6}>
                  <Bosh>Hodisalar ro'yxati bo'sh.</Bosh>
                </td>
              </tr>
            )}
            {qatorlar.map((e, i) => {
              const s = new Date(e.at);
              const yaroqli = !Number.isNaN(s.getTime());
              return (
                <tr
                  key={`${e.at}-${i}`}
                  className="h-[44px] border-b last:border-b-0"
                  style={{ borderColor: HOSHIYA, background: i % 2 === 1 ? ZEBRA : "#fff" }}
                >
                  <td
                    className="truncate px-[6px] pl-[18px] text-[12.5px] font-medium leading-[17px]"
                    style={{ color: IK }}
                  >
                    {yaroqli ? kunSoat(s, hozir) : YOQ}
                  </td>
                  <td
                    className="truncate px-[6px] text-[12.5px] leading-[17px]"
                    style={{ color: e.user ? KUL : XIRA_QUYUQ }}
                    title={e.user}
                  >
                    {e.user || YOQ}
                  </td>
                  <td className="truncate px-[6px] text-[12.5px] leading-[17px]" style={{ color: KUL }}>
                    {e.platform || YOQ}
                  </td>
                  <td className="truncate px-[6px] text-[12.5px] leading-[17px]" style={{ color: KUL }}>
                    {e.app || YOQ}
                  </td>
                  <td className="truncate px-[6px] text-[12.5px] leading-[17px]" style={{ color: KUL }}>
                    {e.network || YOQ}
                  </td>
                  <td
                    className="truncate px-[6px] pr-[18px] text-right text-[12.5px] font-medium leading-[17px]"
                    style={{ color: javobRang(e.status) }}
                    title={e.durationMs ? `${son(e.durationMs)} ms · ${e.requestId ?? ""}` : e.requestId}
                  >
                    {e.status || "—"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Jurnal cheklovini OCHIQ aytamiz: jadvaldagi 5 qator "bor-yo'g'i
          shuncha hodisa bo'lgan" degani emas. */}
      <p
        className="border-t px-[18px] py-[10px] text-[11.5px] leading-4"
        style={{ borderColor: HOSHIYA, color: XIRA_QUYUQ }}
      >
        Jami {son(d.group.count)} ta hodisa · jurnalda oxirgi {son(d.samplesTotal)} tasining to'liq
        namunasi saqlanadi.
      </p>
    </Karta>
  );
}

/* ══ 4 · Amallar tarixi va izohlar ══════════════════════════════════ */

/**
 * Yozuv turining rangi (Figma: uchta 7 px nuqta).
 *
 * Rang — TUR, "yaxshi/yomon" emas: ko'k — holat o'zgardi, to'q sariq —
 * ma'lumot tashqariga chiqdi (Telegram), kulrang — odam yozgan izoh,
 * siyoh — mas'ul biriktirildi, qizil — tizim xatolikni qayta ochdi.
 *
 * `ai` ham to'q sariq: eksport va Telegram kabi u ham ma'lumotni TASHQI
 * xizmatga chiqaradi. Tarixga qarab turib "qaysi yozuvlarda ma'lumot
 * chiqib ketgan" degan savolga bitta rang bilan javob berish kerak.
 */
const NUQTA: Record<string, string> = {
  status: KO_K,
  telegram: ORANJ,
  note: OCH_KUL,
  assign: SIYOH,
  regressed: QIZIL,
  export: XIRA_QUYUQ,
  ai: ORANJ,
};

export function TarixKarta({
  g,
  hozir,
  izohQosh,
}: {
  g: AdminErrorGroup;
  hozir: number;
  izohQosh?: () => void;
}) {
  const yozuvlar = g.activity ?? [];
  return (
    <Karta
      sarlavha="Amallar tarixi va izohlar"
      amal={<KartaAmal nomi="Izoh qo'shish" bosildi={izohQosh} ipucha={izohQosh ? undefined : "Izoh qoldirish uchun ruxsat yo'q"} />}
      tana="px-[18px] py-[16px]"
    >
      {yozuvlar.length === 0 ? (
        <Bosh>Hozircha hech kim bu xatolik ustida amal bajarmagan.</Bosh>
      ) : (
        <div className="flex flex-col gap-[12px]">
          {yozuvlar.map((a, i) => {
            const s = new Date(a.at);
            const yaroqli = !Number.isNaN(s.getTime());
            return (
              <div key={`${a.at}-${i}`} className="flex items-start gap-[10px] pt-[4px]">
                <span
                  aria-hidden
                  className="mt-[5px] h-[7px] w-[7px] shrink-0 rounded-full"
                  style={{ background: NUQTA[a.kind] ?? OCH_KUL }}
                />
                <div className="flex min-w-0 flex-1 flex-col gap-[2px]">
                  <p className="break-words text-[13px] font-medium leading-[18px]" style={{ color: IK }}>
                    {a.text}
                  </p>
                  {/* Ijrochisi bo'sh bo'lsa — amalni TIZIM bajargan. Buni
                      ochiq yozamiz, aks holda qator "kimdir" qilgandek
                      ko'rinadi. */}
                  <p className="break-words text-[12px] leading-4" style={{ color: OCH_KUL }}>
                    {a.actor || "Tizim"}
                  </p>
                </div>
                <span
                  className="w-[96px] shrink-0 text-right text-[12px] leading-4"
                  style={{ color: XIRA_QUYUQ }}
                  title={yaroqli ? kunSoat(s, hozir) : undefined}
                >
                  {yaroqli ? vaqtQisqa(s, hozir) : YOQ}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </Karta>
  );
}

/* ══ 5 · Holat va mas'ul ════════════════════════════════════════════ */

/* ── Bosqichlar tasmasi (Figma 3.12.3 · J · "Bosqichlar") ─────────────
   Nomlar `HOLAT` katalogidan OLINADI, bu yerda qayta yozilmaydi: aks
   holda katalogda "Bartaraf etilmoqda" nomi o'zgargan kuni tasma eski
   so'zni ko'rsatib turardi va admin bitta holatni ikkita deb o'qirdi. */

const BOSQICHLAR: XatoHolat[] = ["new", "watching", "fixing", "resolved"];

/**
 * Joriy holat tasmaning nechanchi bosqichida turibdi.
 *
 * `regressed` tasmada ALOHIDA bosqich emas — u 4-bosqichga yetib, keyin
 * orqaga qaytish. Shuning uchun o'rni 3, lekin doira "yakunlandi" emas,
 * "qaytdi" ko'rinishida chiziladi. `ignored` esa `-1` qaytaradi: sikl
 * to'xtatilgan, bironta bosqich yonmaydi.
 */
function bosqichOrni(h: XatoHolat): number {
  if (h === "regressed") return 3;
  return BOSQICHLAR.indexOf(h);
}

function Bosqichlar({ g }: { g: AdminErrorGroup }) {
  const joriy = bosqichOrni(g.status);
  const qaytdi = g.status === "regressed";
  const xira = g.status === "ignored";

  // Rang KATAKdan emas, ORALIQ raqamidan hisoblanadi: har katakning o'ng
  // yarmi keyingi katakning chap yarmi bilan bir xil rangda bo'lsagina
  // tasma uzluksiz ko'rinadi (ikkalasi ham `chiziq(k)` ni chaqiradi).
  const chiziq = (k: number) =>
    xira || k > joriy ? HOSHIYA : k === joriy && qaytdi ? ORANJ : KO_K;

  return (
    <div className="flex min-w-0 flex-col gap-[10px]">
      <div role="list" aria-label="Xatolik hayot siklining bosqichlari" className="grid grid-cols-4">
        {BOSQICHLAR.map((h, i) => {
          const meta = HOLAT[h];
          const otgan = !xira && i < joriy;
          const faol = !xira && i === joriy;
          const buzuq = faol && qaytdi;
          return (
            <div
              key={h}
              role="listitem"
              // Ekran o'quvchisi uchun joriy bosqich alohida belgilanadi:
              // rang va to'ldirilgan doira faqat KO'ZGA gapiradi.
              aria-current={faol ? "step" : undefined}
              className="flex min-w-0 flex-col items-center gap-[6px]"
            >
              <div className="flex w-full items-center">
                <span
                  className="h-px flex-1"
                  style={{ background: i === 0 ? "transparent" : chiziq(i) }}
                />
                {/* Doira ichida RAQAM (Figma), belgi emas: bosqich nomi
                    ostida turgani uchun raqam "nechanchi qadam" degan
                    savolga bir qarashda javob beradi. */}
                <span
                  aria-hidden
                  className="mx-[3px] grid h-[24px] w-[24px] shrink-0 place-items-center rounded-full text-[11.5px] font-semibold leading-none"
                  style={
                    buzuq
                      ? { background: "#fff", color: ORANJ, boxShadow: `inset 0 0 0 1px ${ORANJ}` }
                      : faol
                        ? { background: meta.rang, color: meta.matn ? IK : "#fff" }
                        : otgan
                          ? { background: AVATAR_FON, color: KO_K }
                          : {
                              background: "#fff",
                              color: XIRA_QUYUQ,
                              boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}`,
                            }
                  }
                >
                  {i + 1}
                </span>
                <span
                  className="h-px flex-1"
                  style={{
                    background: i === BOSQICHLAR.length - 1 ? "transparent" : chiziq(i + 1),
                  }}
                />
              </div>
              <span
                className="w-full break-words text-center text-[10.5px] leading-[14px]"
                style={{
                  color: faol ? IK : otgan ? KUL : XIRA_QUYUQ,
                  fontWeight: faol ? 600 : 500,
                }}
                title={`${meta.nomi} — ${meta.izoh}`}
              >
                {meta.nomi}
              </span>
            </div>
          );
        })}
      </div>

      {/* Regressiya — tasmaning 4-bosqichidan keyingi TARMOQ. Uni tasma
          ichiga beshinchi doira qilib qo'ysak, "shunday bo'lishi kerak"
          degan taassurot qolardi; aslida bu siklning buzilishi. */}
      {qaytdi && (
        <div className="flex min-w-0 flex-col gap-[8px]">
          <div className="flex items-center justify-end gap-[6px]">
            <CornerDownRight size={13} color={ORANJ} aria-hidden />
            <Nishon nomi={HOLAT.regressed.nomi} rang={ORANJ} nuqta />
          </div>
          <Izohcha kor="sariq" ikon={<RotateCcw size={14} aria-hidden />}>
            «Bartaraf etildi» deb belgilangandan keyin xatolik yana kelsa — guruh avtomatik
            «Qayta paydo bo'ldi (regressiya)» holatiga qaytadi, muhimlik bir pog'ona ko'tariladi
            va mas'ul dasturchiga xabar boradi.
          </Izohcha>
        </div>
      )}

      {/* E'tiborsiz qoldirilgan guruhda tasma o'chiq turadi. Sababni SHU
          YERDA ko'rsatamiz: "hech qaysi bosqich yonmayapti" degan savol
          javobsiz qolmasin. Sabab majburiy maydon, lekin eski yozuvlarda
          bo'lmasligi mumkin — u holda "aniqlanmagan". */}
      {xira && (
        <Izohcha ikon={<CircleSlash size={14} aria-hidden />}>
          <span className="font-semibold">Ataylab yopildi</span> — hayot sikli to'xtatilgan,
          hisobotlarga va KPI'ga kirmaydi. Sabab: {g.ignoreReason?.trim() || YOQ}.
        </Izohcha>
      )}
    </div>
  );
}

/* ── Holatga bog'liq maydonlar (Figma 3.12.3 · J · "Namunalar") ───────
   Figma'da bular ALOHIDA kartalar bo'lib chizilgan, bizda esa "Holat va
   mas'ul" kartasining ichidagi quti: ular holatdan kelib chiqadi va
   holat o'zgarishi bilan butunlay almashadi — ikki karta orasida ko'z
   yugurtirishga majbur qilmaslik kerak.

   Yuqoridagi ikki blok BOSHQARUV ("nima qilaman"), bu quti esa DALIL
   ("ish qay ahvolda"). Shuning uchun "Mas'ul dasturchi" qatori tepadagi
   mas'ul qatorini takrorlagandek ko'rinsa ham qoldirildi: tuzatish
   tafsilotlari to'plami yarim qolmasligi kerak. */

const KUN_MS = 86_400_000;

/** Yopilgandan keyingi avtomatik kuzatuv oynasi (Figma: "30 kun davomida yoqilgan"). */
const KUZATUV_KUN = 30;

/** Yaroqsiz yoki bo'sh ISO — `null`. Serverdan buzuq sana kelsa ekran yiqilmasin. */
function payt(iso?: string): Date | null {
  if (!iso) return null;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** "Bugun · 14:21" yoki `undefined` — KQator o'zi "aniqlanmagan" yozadi. */
function paytMatn(iso: string | undefined, hozir: number): string | undefined {
  const d = payt(iso);
  return d ? kunNuqta(d, hozir) : undefined;
}

/** To'liq kunlar farqi; manfiy ham bo'lishi mumkin (b, a dan keyin bo'lsa). */
function kunFarq(a: number, b: number): number {
  return Math.floor((a - b) / KUN_MS);
}

/**
 * "1.4.3 (121)" → "1.4.3".
 *
 * Build raqami solishtirishga xalaqit beradi: veb'da u SHA (`a1c3f9d`),
 * mobilda son (`118`) — bitta versiya ikki xil yozilgan bo'ladi.
 */
function asosVersiya(v?: string): string {
  return (v ?? "").replace(/\s*\(.*\)\s*$/, "").trim();
}

/** KQator ga uzatiladigan tayyor katak. `qiymat` bo'sh bo'lsa — "aniqlanmagan". */
type Katak = { qiymat?: string; rang?: string; ipucha?: string };

/**
 * "Tekshiruv" qatori — yopilgandan keyin xatolik qaytdimi.
 *
 * Bu yerda eng katta xato — YOLG'ON TINCHLIK: `lastSeenAt` `resolvedAt`
 * dan keyin bo'lsa, guruh "Bartaraf etildi" deb tursa ham xatolik davom
 * etyapti (tizim regressiyani hali belgilamagan). Shuning uchun bu holat
 * qizil rangda ochiq aytiladi.
 *
 * Tinch holatda sanoq OXIRGI HODISAdan yuritiladi, yopilish sanasidan
 * emas: admin "hozir tinchmi?" deb so'raydi, "yopilgunga qadar tinch
 * edimi?" deb emas. Ikkala sana ipuchada baribir ko'rinadi.
 */
function tekshiruv(g: AdminErrorGroup, hozir: number): Katak {
  const yopilgan = payt(g.resolvedAt);
  const oxirgi = payt(g.lastSeenAt);
  if (!yopilgan || !oxirgi) return {};
  const ipucha = `Oxirgi hodisa ${kunNuqta(oxirgi, hozir)} · yopilgan ${kunNuqta(yopilgan, hozir)}`;
  if (oxirgi.getTime() > yopilgan.getTime()) {
    return {
      qiymat: `Yopilgandan KEYIN ham hodisa kelgan — ${kunNuqta(oxirgi, hozir)}`,
      rang: QIZIL,
      ipucha,
    };
  }
  const kun = kunFarq(hozir, oxirgi.getTime());
  if (kun < 1) return { qiymat: "Bir kun ham o'tmadi — xulosa qilish erta", rang: ORANJ, ipucha };
  return { qiymat: `${son(kun)} kundan beri takrorlanmadi`, rang: YASHIL, ipucha };
}

/**
 * "Eski versiyalarda" qatori — tuzatish chiqqan bo'lsa ham, yangilanmagan
 * qurilmalarda xatolik davom etadi. Ta'sir taqsimotidagi (oxirgi 30 kun)
 * versiyalar tuzatilgan versiya bilan solishtiriladi.
 */
function eskiVersiyalar(d: AdminErrorDetail): Katak {
  const fix = asosVersiya(d.group.fixedVersion);
  // Tuzatilgan versiya yozilmagan — solishtiradigan narsa yo'q, qator
  // "aniqlanmagan" bo'lib turishi TO'G'RI.
  if (!fix) return {};
  const app = d.impact?.app ?? [];
  // Bo'sh taqsimot esa boshqa gap. Hodisalar 30 kunlik TTL bilan
  // o'chiriladi, ya'ni bartaraf etilgan va o'shandan beri tinch turgan
  // guruhda `impact.app` bo'shab qolishi ODATIY hol. Bu eng YAXSHI natija;
  // uni "aniqlanmagan" deb ko'rsatish adminni yo'q muammoni qidirishga
  // majbur qilardi.
  if (app.length === 0) {
    return {
      qiymat: "Eski versiyalarda uchramaydi",
      rang: YASHIL,
      ipucha: "Oxirgi 30 kunda umuman hodisa qayd etilmagan.",
    };
  }
  const eski: string[] = [];
  for (const u of app) {
    // Qoldiq qatori versiya EMAS, bir necha mayda versiyaning yig'indisi —
    // uni ro'yxatga qo'shsak "Hali Boshqa da uchraydi" degan ma'nosiz matn
    // chiqardi. Bayroq bo'yicha tekshiramiz: yorliq matni o'zgarsa, kalit
    // bo'yicha tekshiruv jimgina buzilardi.
    if (u.other) continue;
    const v = asosVersiya(u.key);
    if (v && v !== fix && !eski.includes(v)) eski.push(v);
  }
  const ipucha = `Oxirgi 30 kunlik ta'sir taqsimoti ${fix} versiyasi bilan solishtirildi.`;
  if (eski.length === 0) return { qiymat: "Eski versiyalarda uchramaydi", rang: YASHIL, ipucha };
  // Ikkitadan ortig'i qatorga sig'maydi — qolganini sanab aytamiz.
  const korinadi = eski.slice(0, 2).join(", ");
  const qolgan = eski.length - 2;
  return {
    qiymat:
      qolgan > 0
        ? `Hali ${korinadi} va yana ${son(qolgan)} ta versiyada uchraydi`
        : `Hali ${korinadi} da uchraydi`,
    rang: ORANJ,
    ipucha: `${ipucha} Uchraydigan versiyalar: ${eski.join(", ")}.`,
  };
}

/** "Avtomatik kuzatuv" qatori — yopilish sanasidan 30 kun. */
function kuzatuv(g: AdminErrorGroup, hozir: number): Katak {
  const yopilgan = payt(g.resolvedAt);
  if (!yopilgan) return {};
  const otgan = kunFarq(hozir, yopilgan.getTime());
  const qolgan = KUZATUV_KUN - otgan;
  if (qolgan <= 0) {
    return {
      qiymat: `Tugagan — yopilganiga ${son(otgan)} kun bo'ldi`,
      rang: OCH_KUL,
      ipucha: `${KUZATUV_KUN} kunlik oyna yopildi: bundan keyin xatolik qaytsa yangi guruh ochiladi.`,
    };
  }
  return {
    qiymat: `${KUZATUV_KUN} kun davomida yoqilgan · yana ${son(qolgan)} kun`,
    ipucha: `Kuzatuv ${kunNuqta(new Date(yopilgan.getTime() + KUZATUV_KUN * KUN_MS), hozir)} gacha ishlaydi.`,
  };
}

/** Maydonlar qutisining boshi — Figma namunasidagidek: holat nishoni + guruh kodi. */
function MaydonQuti({ g, children }: { g: AdminErrorGroup; children: ReactNode }) {
  const meta = HOLAT[g.status];
  return (
    <div
      className="flex min-w-0 flex-col rounded-[10px] px-[12px] pb-[8px]"
      style={{ background: QUTI_FON, boxShadow: `inset 0 0 0 1px ${HOSHIYA}` }}
    >
      <div
        className="flex h-[38px] shrink-0 items-center justify-between gap-2"
        style={{ boxShadow: `inset 0 -1px 0 0 ${HOSHIYA}` }}
      >
        <Nishon nomi={meta.nomi} rang={meta.rang} matn={meta.matn} nuqta />
        <span className="truncate text-[11.5px] font-medium leading-4" style={{ color: XIRA_QUYUQ }}>
          {g.ref}
        </span>
      </div>
      {children}
    </div>
  );
}

function HolatMaydonlar({ d, hozir }: { d: AdminErrorDetail; hozir: number }) {
  const g = d.group;

  if (g.status === "fixing") {
    return (
      <MaydonQuti g={g}>
        <KQator yorliq="Mas'ul dasturchi" qiymat={g.assignee} ipucha={g.assignee} />
        <KQator yorliq="Boshlangan" qiymat={paytMatn(g.startedAt, hozir)} />
        <KQator yorliq="Rejalashtirilgan versiya" qiymat={g.plannedVersion} />
        {/* Tuzatish izohi bo'lmasa umumiy izohga tushamiz: dasturchi
            ko'pincha alohida maydon o'rniga oddiy izoh qoldiradi. */}
        <KQator
          yorliq="Tuzatish izohi"
          qiymat={g.fixNote || g.note}
          ipucha={g.fixNote || g.note}
        />
        <KQator yorliq="Oxirgi hodisa" qiymat={paytMatn(g.lastSeenAt, hozir)} />
        {/* `sinceStarted` — `startedAt` dan KEYINGI hodisalar soni. Umumiy
            sanoq bu savolga javob bermaydi: u ish boshlangunga qadar
            to'plangan hodisalarni ham o'z ichiga oladi. */}
        <KQator
          yorliq="Boshlanganidan beri"
          qiymat={d.sinceStarted !== undefined ? `${son(d.sinceStarted)} ta yangi hodisa` : undefined}
          rang={d.sinceStarted !== undefined && d.sinceStarted > 0 ? ORANJ : undefined}
          ipucha="Tuzatish boshlangan paytdan keyin qayd etilgan hodisalar."
          oxirgi
        />
      </MaydonQuti>
    );
  }

  if (g.status === "resolved") {
    const t = tekshiruv(g, hozir);
    const e = eskiVersiyalar(d);
    const k = kuzatuv(g, hozir);
    return (
      <MaydonQuti g={g}>
        {/* Tuzatgan odam yozilmagan bo'lsa mas'ulga tushamiz: guruhni
            odatda o'zi biriktirilgan dasturchi yopadi. */}
        <KQator yorliq="Tuzatgan" qiymat={g.resolvedBy || g.assignee} ipucha={g.resolvedBy || g.assignee} />
        <KQator yorliq="Tuzatilgan versiya" qiymat={g.fixedVersion || g.closedVersion} />
        <KQator yorliq="Yopilgan sana" qiymat={paytMatn(g.resolvedAt, hozir)} />
        <KQator yorliq="Tekshiruv" qiymat={t.qiymat} rang={t.rang} ipucha={t.ipucha} qalin />
        <KQator yorliq="Eski versiyalarda" qiymat={e.qiymat} rang={e.rang} ipucha={e.ipucha} />
        <KQator yorliq="Avtomatik kuzatuv" qiymat={k.qiymat} rang={k.rang} ipucha={k.ipucha} oxirgi />
      </MaydonQuti>
    );
  }

  if (g.status === "regressed") {
    return (
      <MaydonQuti g={g}>
        {/* `closedVersion` eski yozuvlarda yo'q — o'shanda tuzatilgan
            versiyani ko'rsatamiz, ikkisi amalda bir xil chiqishni
            bildiradi (ipuchada ochiq aytilgan). */}
        <KQator
          yorliq="Oldin yopilgan versiya"
          qiymat={g.closedVersion || g.fixedVersion}
          ipucha={
            g.closedVersion
              ? "Guruh shu versiyada yopilgan edi."
              : "Yopilish versiyasi yozilmagan — tuzatilgan versiya ko'rsatilmoqda."
          }
        />
        <KQator yorliq="Qachon qaytgan" qiymat={paytMatn(g.reopenedAt, hozir)} rang={ORANJ} />
        <KQator yorliq="Qaysi versiyada qaytgan" qiymat={g.lastAppVersion} rang={ORANJ} oxirgi />
      </MaydonQuti>
    );
  }

  // `new`, `watching`, `ignored` — hayot siklining qo'shimcha maydonlari
  // hali (yoki umuman) yo'q. Bo'sh quti chizmaymiz: "aniqlanmagan" bilan
  // to'la blok kartani shovqinga to'ldirardi, ignored sababi esa tasma
  // ostida allaqachon aytilgan.
  return null;
}

export function HolatKarta({
  d,
  hozir,
  ozgartirsaBoladi,
  holatBosildi,
  masulBosildi,
}: {
  d: AdminErrorDetail;
  hozir: number;
  ozgartirsaBoladi: boolean;
  holatBosildi: (h: XatoHolat) => void;
  masulBosildi: () => void;
}) {
  const g = d.group;
  // `regressed` — TIZIM belgisi, qo'lda tanlanmaydi. Lekin JORIY holat
  // shu bo'lsa, u ko'rinishi SHART: aks holda kartada hech bir tugma
  // yonmay turadi va admin holatni umuman ko'rmaydi.
  const qolda = HOLAT_TARTIBI as XatoHolat[];
  const tanlovlar: XatoHolat[] = qolda.includes(g.status) ? qolda : [g.status, ...qolda];

  return (
    <Karta sarlavha="Holat va mas'ul" tana="px-[18px] pb-[18px] pt-[16px]">
      <div className="flex flex-col gap-[14px]">
        {/* Tasma tugmalardan YUQORIDA: admin avval "hozir qaysi bosqichda
            turibmiz" ni ko'rsin, keyin "qayerga o'tkazaman" degan tanlovni
            qilsin (Figma 3.12.3 · J). */}
        <Bosqichlar g={g} />

        <div className="flex flex-col gap-[8px]">
          <p className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
            Holat
          </p>
          <div className="flex flex-wrap gap-[6px]">
            {tanlovlar.map((h) => {
              const meta = HOLAT[h];
              const faol = g.status === h;
              const tizim = !qolda.includes(h);
              const ochiq = !ozgartirsaBoladi || tizim || faol;
              return (
                <button
                  key={h}
                  type="button"
                  onClick={() => holatBosildi(h)}
                  disabled={ochiq}
                  aria-pressed={faol}
                  title={
                    tizim
                      ? `${meta.nomi} — ${meta.izoh}`
                      : ozgartirsaBoladi
                        ? `${meta.nomi} — ${meta.izoh}`
                        : "Faqat superadmin holatni o'zgartira oladi."
                  }
                  className={`inline-flex h-[28px] shrink-0 items-center justify-center rounded-full px-[11px] text-[11.5px] font-medium leading-4 transition-colors ${
                    ochiq ? "cursor-default" : "hover:bg-[#f4f6fc]"
                  } ${FOKUS}`}
                  style={
                    faol
                      ? { background: meta.rang, color: meta.matn ? IK : "#fff" }
                      : {
                          background: "#fff",
                          color: ozgartirsaBoladi ? KUL : XIRA_QUYUQ,
                          boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}`,
                        }
                  }
                >
                  {meta.qisqa}
                </button>
              );
            })}
          </div>
        </div>

        <div className="flex flex-col gap-[8px]">
          <p className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
            Mas'ul admin
          </p>
          <div
            className="flex h-[40px] items-center justify-between gap-2 rounded-[8px] pl-[10px] pr-[12px]"
            style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}` }}
          >
            <div className="flex min-w-0 items-center gap-[9px]">
              {g.assignee ? (
                <>
                  <Avatar nomi={g.assignee} olcham={26} fon={AVATAR_KO_K} rang={KO_K} />
                  <span
                    className="truncate text-[12.5px] font-medium leading-[17px]"
                    style={{ color: IK }}
                    title={g.assignee}
                  >
                    {g.assignee}
                  </span>
                </>
              ) : (
                <span className="truncate text-[12.5px] leading-[17px]" style={{ color: XIRA_QUYUQ }}>
                  Biriktirilmagan
                </span>
              )}
            </div>
            <button
              type="button"
              onClick={masulBosildi}
              disabled={!ozgartirsaBoladi}
              title={ozgartirsaBoladi ? undefined : "Faqat superadmin mas'ul biriktira oladi."}
              className={`shrink-0 rounded-md text-[12px] font-medium leading-4 transition-opacity ${
                ozgartirsaBoladi ? "hover:opacity-70" : "cursor-default"
              } ${FOKUS}`}
              style={{ color: ozgartirsaBoladi ? KO_K : XIRA_QUYUQ }}
            >
              {g.assignee ? "O'zgartirish" : "Biriktirish"}
            </button>
          </div>
        </div>

        <HolatMaydonlar d={d} hozir={hozir} />
      </div>
    </Karta>
  );
}

/* ══ 6 · Qurilma va muhit ═══════════════════════════════════════════
   Figma 3.12.3 · H. Karta PLATFORMAGA QARAB shoxlanadi: uchta klient —
   uchta maydonlar to'plami.

   Bitta umumiy ro'yxat chizib bo'lmaydi. iOS'da "API level", veb'da
   "Flutter / Dart" hech qachon to'lmaydi; ular umumiy ro'yxatda doim
   bo'sh turardi va admin bo'sh qatorni "yig'ilmagan" deb o'qib, yo'q
   nosozlikni qidirib ketardi. */

/** Bo'sh katakning sababi. Matnlar `xato.ts` dagi uchta konstantadan. */
type BoshTur = "yigilmagan" | "nomalum" | "tegishsiz";

const BOSH_MATN: Record<BoshTur, string> = {
  yigilmagan: YOQ,
  nomalum: NOMALUM,
  tegishsiz: TEGISHSIZ,
};

/**
 * Har bir bo'shliq turining SABABI.
 *
 * Ipuchada ochiq aytiladi, chunki "—" va "ma'lum emas" o'z-o'zidan
 * tushunarli emas: admin ularni ham nosozlik deb o'ylab, dasturchiga
 * "nega bo'sh?" degan savol bilan borishi mumkin.
 */
const BOSH_IZOH: Record<BoshTur, string> = {
  yigilmagan:
    "Maydon bu platformada mavjud, lekin klient uni hali yubormaydi — X-Client-Device sarlavhasi qo'shilishi kerak.",
  nomalum: "Manba bu maydonni bermaydi — brauzer uni oshkor qilmaydi.",
  tegishsiz: "Bu maydon shu platformaga tegishli emas.",
};

/**
 * Muhit kartasining bitta qatori.
 *
 * NEGA `KQator` ustiga o'ram kerak: `KQator` bo'sh qiymatni FAQAT bitta
 * so'z bilan ("aniqlanmagan") yozadi. Tayyor matnni to'g'ridan-to'g'ri
 * `qiymat` ga uzatib ham bo'lmaydi — u holda "—" haqiqiy qiymat kabi
 * QORA bo'lib chiqadi va ko'z uni ma'lumot deb o'qiydi. Shuning uchun
 * rang majburan xira qilinadi.
 */
function MQator({
  yorliq,
  qiymat,
  bosh = "yigilmagan",
  oxirgi,
}: {
  yorliq: string;
  qiymat?: string;
  bosh?: BoshTur;
  oxirgi?: boolean;
}) {
  const val = (qiymat ?? "").trim();
  if (val) {
    // Ipucha qiymatning o'zi: tor ustunda uzun satr ("1080 × 2400 ·
    // 2.75x") o'ralib ketadi, sichqoncha ostida esa butun holda ko'rinadi.
    return <KQator yorliq={yorliq} qiymat={val} ipucha={val} oxirgi={oxirgi} />;
  }
  return (
    <KQator
      yorliq={yorliq}
      // "aniqlanmagan" ni KQator O'ZI yozadi — takrorlamaymiz, aks holda
      // yozuv ikki joyda tuzatilishi kerak bo'lardi.
      qiymat={bosh === "yigilmagan" ? undefined : BOSH_MATN[bosh]}
      rang={XIRA_QUYUQ}
      ipucha={BOSH_IZOH[bosh]}
      oxirgi={oxirgi}
    />
  );
}

type PlatTur = "android" | "ios" | "web" | "boshqa";

const PLAT_NOMI: Record<PlatTur, string> = {
  android: "Android",
  ios: "iOS",
  web: "Web (brauzer)",
  boshqa: "Qurilmasiz",
};

/**
 * Maydonlar to'plamini tanlaydigan platforma.
 *
 * AVVAL `platform` ga ishonamiz. U XOM sarlavha emas: server uni YOPIQ
 * ro'yxatga tekshiradi (errlog/device.go · ParseDevice — android | ios |
 * web | bo'sh), begona qiymat esa umuman tashlanadi. Ya'ni bu maydon
 * bo'sh bo'lishi mumkin, lekin YOLG'ON bo'la olmaydi.
 *
 * NEGA `os` ga qarab qaror qilib bo'lmaydi: veb mijoz uchun ham `os`
 * User-Agent'dan to'ldiriladi (device.go · fillFromUA → uaOS) va u
 * "Android" yoki "iOS" bo'lishi mumkin. Eski tartibda mobil brauzerdagi
 * veb xatoligi Android kartasi bilan chizilardi: "Model", "API level",
 * "Flutter / Dart" qatorlari bo'sh turib nosozlik signalini berardi,
 * to'ldirilgan "Brauzer" va "Brauzer dvigateli" esa umuman ko'rinmasdi.
 *
 * Evristika faqat `platform` BO'SH bo'lganda ishlaydi — eski yozuvlarda
 * sarlavha yo'q edi, lekin brauzer nomi User-Agent'dan ajratib olingan.
 * Shuning uchun evristika ichida ham `os` dan OLDIN brauzer belgisi
 * tekshiriladi: `os` ikki tomonga ham ishlaydi, brauzer dvigateli esa
 * faqat rost brauzerda to'ladi.
 *
 * NEGA `browser` ning o'zi yetarli emas: Dio'ning User-Agent'i
 * "Dart/3.x (dart:io)" bo'lgani uchun `uaBrowser` mobil ilova so'rovida ham
 * "Dart 3" qaytaradi (device.go · uaProducts). Ya'ni sarlavhasi chala kelgan
 * Flutter ilovasi "veb" deb o'qilib qolardi. Dvigatel (`engine`) esa faqat
 * AppleWebKit/Gecko topilganda to'ladi, Flutter/Dart versiyalari borligi ham
 * ilova ekanini aniq ko'rsatadi.
 */
function platTur(q?: XatoQurilma): PlatTur {
  const p = (q?.platform ?? "").trim().toLowerCase();
  if (p === "web" || p === "android" || p === "ios") return p;

  const ilova = !!(q?.flutter || q?.dart);
  const brauzer = (q?.browser ?? "").trim();
  const rostBrauzer =
    !ilova && (!!q?.engine || (brauzer !== "" && !/^dart\b/i.test(brauzer)));
  if (rostBrauzer) return "web";

  const s = `${p} ${q?.os ?? ""}`.toLowerCase();
  if (s.includes("android")) return "android";
  if (s.includes("ios") || s.includes("iphone") || s.includes("ipad")) return "ios";
  if (s.includes("web") || brauzer !== "") return "web";
  return "boshqa";
}

function platIkon(t: PlatTur) {
  if (t === "android") return <Smartphone size={15} color={KO_K} aria-hidden />;
  if (t === "ios") return <Apple size={15} color={KO_K} aria-hidden />;
  if (t === "web") return <Globe size={15} color={KO_K} aria-hidden />;
  return <Server size={15} color={KO_K} aria-hidden />;
}

/** Bitta qatorning tavsifi — ro'yxat JSX'dan OLDIN ma'lumot bo'lib yig'iladi. */
type MMaydon = { yorliq: string; qiymat?: string; bosh?: BoshTur };

/**
 * Platformaga mos maydonlar ro'yxati (Figma 3.12.3 · H · uchta karta).
 *
 * Ro'yxat JSX ichida `t === "..."` shartlari bilan chizilmaydi: uchta
 * to'plam SHU YERDA yonma-yon turgani uchun ularning farqi (API level
 * qayerda "—", emulyator qatori qanday nomlanadi) bir qarashda ko'rinadi
 * va yangi klient qo'shilsa ish bitta massiv qo'shishga qisqaradi.
 */
function muhitMaydonlar(t: PlatTur, q: XatoQurilma | undefined, g: AdminErrorGroup): MMaydon[] {
  const osi = birik(" ", q?.os, q?.osVersion);
  // Namunada versiya bo'lmasa guruhning oxirgi versiyasiga tushamiz: eski
  // yozuvlarda `device` bo'sh, `lastAppVersion` esa bor.
  const ilova = q?.appVersion
    ? `${q.appVersion}${q.build ? ` (${q.build})` : ""}`
    : g.lastAppVersion;

  if (t === "web") {
    return [
      { yorliq: "Ishlab chiqaruvchi", qiymat: q?.brand },
      { yorliq: "Brauzer", qiymat: q?.browser },
      { yorliq: "Brauzer dvigateli", qiymat: q?.engine },
      { yorliq: "OS versiyasi", qiymat: osi },
      // Veb'da Android API level tushunchasi umuman yo'q.
      { yorliq: "API level", bosh: "tegishsiz" },
      { yorliq: "Veb build", qiymat: ilova },
      // Next.js versiyasini server BILMAYDI: u `XatoQurilma` da yo'q va
      // `X-Client-Device` sarlavhasida ham yuborilmaydi. "yigilmagan"
      // aynan shuni aytadi — maydon veb uchun mavjud, klient hali
      // to'ldirmaydi (Figma 3.12.3 · H · veb kartasi).
      { yorliq: "Next.js", bosh: "yigilmagan" },
      { yorliq: "Ekran · oyna", qiymat: q?.screen },
      // Brauzer RAM, xotira va batareyani standart yo'l bilan bermaydi
      // (`deviceMemory` va Battery API faqat Chromium'da, u ham
      // yaxlitlangan). Ya'ni bu "yig'ilmagan" emas — "olinmaydi". Qiymat
      // baribir kelsa ko'rsatamiz: Chromium'da u haqiqiy son.
      { yorliq: "RAM", qiymat: q?.ram, bosh: "nomalum" },
      { yorliq: "Xotira", qiymat: q?.storage, bosh: "nomalum" },
      { yorliq: "Til · mintaqa", qiymat: q?.locale },
      { yorliq: "Tarmoq", qiymat: q?.network },
      { yorliq: "Batareya", qiymat: q?.battery, bosh: "nomalum" },
      // "Sahifa" va "Referrer" — brauzerdagi SAHIFA yo'li. Namunadagi
      // `path` bu emas: u so'rov yuborilgan API yo'li ("/api/admin/users")
      // va u "So'rov ma'lumotlari" kartasida alohida ko'rsatiladi. Uni shu
      // yerga qo'yish adminni "xatolik shu sahifada bo'lgan" degan noto'g'ri
      // xulosaga olib borardi, shuning uchun rostini aytamiz: yig'ilmagan.
      { yorliq: "Sahifa", bosh: "yigilmagan" },
      { yorliq: "Referrer", bosh: "yigilmagan" },
      // Orientatsiya ATAYLAB yo'q: u faqat Android va iOS kartalarida
      // (Figma 3.12.3 · H) — brauzerda ekran o'lchami "Ekran · oyna"
      // qatoridan o'qiladi.
    ];
  }

  // Android va iOS to'plamlari BIR XIL tartibda, ikkita farq bilan: API
  // level (iOS'da bunday tushuncha yo'q) va emulyator qatorining nomi.
  const ios = t === "ios";
  return [
    { yorliq: "Ishlab chiqaruvchi", qiymat: q?.brand },
    { yorliq: "Model", qiymat: q?.model },
    { yorliq: "Model kodi", qiymat: q?.modelCode },
    { yorliq: "OS versiyasi", qiymat: osi },
    ios ? { yorliq: "API level", bosh: "tegishsiz" } : { yorliq: "API level", qiymat: q?.apiLevel },
    { yorliq: "Ilova versiyasi", qiymat: ilova },
    { yorliq: "Flutter / Dart", qiymat: birik(" / ", q?.flutter, q?.dart) },
    { yorliq: "Ekran", qiymat: q?.screen },
    { yorliq: "RAM", qiymat: q?.ram },
    { yorliq: "Xotira", qiymat: q?.storage },
    { yorliq: "Til · mintaqa", qiymat: q?.locale },
    { yorliq: "Tarmoq", qiymat: q?.network },
    { yorliq: "Batareya", qiymat: q?.battery },
    { yorliq: ios ? "Simulyator · Jailbreak" : "Emulyator · Root", qiymat: q?.emulator },
    { yorliq: "Orientatsiya", qiymat: q?.orientation },
  ];
}

export function MuhitKarta({ d }: { d: AdminErrorDetail }) {
  const q = d.sample?.device;
  const qurilma = d.sample?.deviceLabel || qurilmaYorliq(q) || d.group.lastDevice;
  /*
   * "Namuna yo'q" va "qurilma yo'q" — IKKI XIL hol, ularni aralashtirib
   * bo'lmaydi.
   *
   * Namunalar bazada 30 kunlik TTL bilan o'chiriladi (pkg/db/indexes.go ·
   * error_samples · at_ttl), guruh esa qoladi. Ya'ni 30 kundan eski HAR
   * QANDAY mobil xatolik uchun `d.sample` bo'sh keladi — eski tartibda
   * karta bunga qarab "xatolik server tomonda yuz bergan" deb YOLG'ON
   * sabab yozardi, ustiga `lastDevice` (unda TTL yo'q) va `lastAppVersion`
   * ekranda umuman ko'rinmasdi.
   *
   * Shuning uchun `lastDevice` bor bo'lsa alohida, "eskirgan" ko'rinish
   * chiziladi: qurilma MA'LUM, faqat batafsil maydonlar o'chirilgan.
   */
  const eskirgan = !d.sample && !!d.group.lastDevice;
  // Eskirgan holatda tuzilmali qurilma qolmagan — platformani tayyor
  // yorliqdan ("Xiaomi Redmi Note 12 · Android 14") topamiz: uni `os`
  // sifatida bersak, platTur ning eski yozuvlar uchun mo'ljallangan
  // evristikasi ishlaydi.
  const t = eskirgan ? platTur({ os: d.group.lastDevice }) : platTur(q);
  const maydonlar = eskirgan || t === "boshqa" ? [] : muhitMaydonlar(t, q, d.group);

  return (
    <Karta sarlavha="Qurilma va muhit" tana="px-[18px] pb-[14px] pt-[10px]">
      <div className="flex min-w-0 flex-col gap-[12px]">
        {!eskirgan && t === "boshqa" ? (
          // Fon jarayoni va server panikasida mijoz ham, qurilma ham
          // bo'lmaydi. Bunda 15 ta "aniqlanmagan" qator chizish yolg'on
          // signal berardi: ma'lumot yo'qolmagan, u umuman mavjud emas.
          <Bosh>
            Qurilma ma'lumoti yozilmagan — xatolik server tomonda yoki fon jarayonida yuz bergan.
          </Bosh>
        ) : (
          <div className="flex min-w-0 flex-col">
            {/* Platforma nomi maydonlar USTIDA: ro'yxat qaysi to'plam
                ekani — nega "API level" bor yoki "—" — shu qatordan
                o'qiladi. Ostidagi kichik satr qurilmaning to'liq
                yorlig'i: avvalgi "Platforma" va "Qurilma" qatorlari
                shu ikki satrga yig'ildi. */}
            <div className="flex min-w-0 items-center gap-[9px] pb-[10px]">
              <span
                aria-hidden
                className="grid h-[28px] w-[28px] shrink-0 place-items-center rounded-[8px]"
                style={{ background: AVATAR_FON }}
              >
                {platIkon(t)}
              </span>
              <div className="flex min-w-0 flex-col">
                <span
                  className="truncate text-[13px] font-semibold leading-[18px]"
                  style={{ color: IK }}
                >
                  {/* Yorliqdan platforma o'qilmasa ("Chrome 128 · Windows 11"
                      da na "web", na "android" so'zi bor) "Qurilmasiz" deb
                      yozib bo'lmaydi — ostida qurilma nomi turibdi. */}
                  {eskirgan && t === "boshqa" ? "Platforma aniqlanmagan" : PLAT_NOMI[t]}
                </span>
                <span
                  className="truncate text-[11.5px] leading-[15px]"
                  style={{ color: qurilma ? OCH_KUL : XIRA_QUYUQ }}
                  title={qurilma}
                >
                  {qurilma || YOQ}
                </span>
              </div>
            </div>
            {eskirgan ? (
              <>
                {/* Guruhda TTL'siz saqlanadigan yagona batafsil maydon. */}
                <MQator yorliq="Ilova versiyasi" qiymat={d.group.lastAppVersion} oxirgi />
                <div className="pt-[10px]">
                  <Izohcha ikon={<CircleSlash size={14} aria-hidden />}>
                    Namuna saqlash muddati (30 kun) tugagan — batafsil maydonlar o'chirilgan.
                    Qurilma va ilova versiyasi guruhning o'zida saqlanadi, shuning uchun ular
                    qoldi.
                  </Izohcha>
                </div>
              </>
            ) : (
              maydonlar.map((m, i) => (
                <MQator
                  key={m.yorliq}
                  yorliq={m.yorliq}
                  qiymat={m.qiymat}
                  bosh={m.bosh}
                  oxirgi={i === maydonlar.length - 1}
                />
              ))
            )}
          </div>
        )}

        {/* ── Server muhiti ───────────────────────────────────────────
            Bu uchta qator QURILMAGA tegishli emas: ular hodisani qabul
            qilgan server haqida va platforma qanday bo'lsa ham
            o'zgarmaydi. Yuqoridagi ro'yxat ichida qolsa, admin "Backend
            versiyasi" ni telefondagi ilova versiyasi deb o'qishi mumkin —
            shuning uchun alohida quti (QUTI_FON "kartaning ichki bo'lagi"
            degan ma'noni beradi). */}
        <div
          className="flex min-w-0 flex-col rounded-[10px] px-[12px] pb-[8px]"
          style={{ background: QUTI_FON, boxShadow: `inset 0 0 0 1px ${HOSHIYA}` }}
        >
          <div
            className="flex h-[34px] shrink-0 items-center gap-[7px]"
            style={{ boxShadow: `inset 0 -1px 0 0 ${HOSHIYA}` }}
          >
            <Server size={13} color={OCH_KUL} className="shrink-0" aria-hidden />
            <span className="truncate text-[11.5px] font-semibold leading-4" style={{ color: KUL }}>
              Server muhiti
            </span>
          </div>
          <KQator yorliq="Muhit (APP_ENV)" qiymat={d.env?.appEnv} />
          <KQator yorliq="Server" qiymat={d.env?.server} ipucha={d.env?.server} />
          {/* Binar `-ldflags` siz yig'ilsa bu maydon bo'sh keladi — "aniqlanmagan"
              yozuvi KQator ning o'zida. */}
          <KQator yorliq="Backend versiyasi" qiymat={d.env?.version} oxirgi />
        </div>
      </div>
    </Karta>
  );
}

/* ══ 7 · So'rov ma'lumotlari ════════════════════════════════════════ */

/**
 * HTTP kodining serverdagi xato kodi (`APIError.code`) bilan juftligi.
 * Figma "500 · internal" ko'rinishida ko'rsatadi: admin javob kodini ham,
 * uning MA'NOSINI ham bir qatorda o'qisin.
 */
const JAVOB_NOMI: Record<number, string> = {
  400: "validation",
  401: "unauthorized",
  403: "forbidden",
  404: "not_found",
  409: "conflict",
  422: "validation",
  429: "rate_limited",
  500: "internal",
  502: "bad_gateway",
  503: "server_unavailable",
  504: "timeout",
};

/** Sekin so'rov — sariq, juda sekin — qizil. Chegara: 1 s va 5 s. */
function davomiylikRang(ms?: number): string | undefined {
  if (!ms) return undefined;
  if (ms >= 5000) return QIZIL;
  if (ms >= 1000) return ORANJ;
  return undefined;
}

export function SorovKarta({ d, nusxala }: { d: AdminErrorDetail; nusxala: () => void }) {
  const s = d.sample;
  const javob =
    s?.status !== undefined
      ? `${s.status}${JAVOB_NOMI[s.status] ? ` · ${JAVOB_NOMI[s.status]}` : ""}`
      : undefined;

  return (
    <Karta
      sarlavha="So'rov ma'lumotlari"
      amal={
        <KartaAmal
          nomi="Nusxa olish"
          bosildi={s ? nusxala : undefined}
          ipucha={s ? "So'rov ma'lumotlarini buferga oladi" : "Nusxalash uchun namuna yo'q"}
        />
      }
      tana="px-[18px] pb-[10px] pt-[4px]"
    >
      {!s ? (
        <Bosh>Bu xatolik HTTP so'rovi bilan bog'liq emas yoki namuna saqlanmagan.</Bosh>
      ) : (
        <>
          <KQator
            yorliq="Metod va endpoint"
            qiymat={birik(" ", s.method, s.path) || d.group.path}
            ipucha={s.path ?? d.group.path}
          />
          <KQator
            yorliq="Javob kodi"
            qiymat={javob}
            rang={s.status !== undefined ? javobRang(String(s.status)) : undefined}
          />
          <KQator yorliq="requestId" qiymat={s.requestId} />
          <KQator
            yorliq="Admin"
            qiymat={birik(" ", s.actor, s.actorRole ? `(${s.actorRole})` : undefined)}
          />
          {/* IP manzil jurnalda SAQLANMAYDI (`internal/errlog/scrub.go`).
              Qatorni olib tashlamaymiz: "yozilmaydi" ham javob. */}
          <KQator
            yorliq="IP manzil"
            ipucha="Server IP manzilni xatoliklar jurnaliga yozmaydi."
          />
          <KQator yorliq="Mijoz platformasi" qiymat={s.device.platform} />
          <KQator
            yorliq="Davomiylik"
            qiymat={s.durationMs ? `${son(s.durationMs)} ms` : undefined}
            rang={davomiylikRang(s.durationMs)}
            oxirgi
          />
        </>
      )}
    </Karta>
  );
}

/* ══ 8 · Ta'sirlangan foydalanuvchilar ══════════════════════════════ */

export function OdamlarKarta({ d }: { d: AdminErrorDetail }) {
  const [hammasi, setHammasi] = useState(false);
  const KOR = 3; // Figma: uchta qator.
  const qatorlar = hammasi ? d.users : d.users.slice(0, KOR);
  const yashirin = d.users.length - KOR;
  // Jurnal noyob hash'larni sanaydi, ro'yxat esa faqat NOMI ma'lum
  // bo'lganlarni ko'rsatadi — ikki son teng bo'lmasligi normal.
  const nomalum = Math.max(0, d.group.usersCount - d.users.length);

  return (
    <Karta
      sarlavha="Ta'sirlangan foydalanuvchilar"
      amal={
        yashirin > 0 ? (
          <KartaAmal
            nomi={hammasi ? "Kamroq" : `Barchasi (${son(d.users.length)})`}
            bosildi={() => setHammasi((v) => !v)}
          />
        ) : (
          <span className="shrink-0 text-[12px] font-medium leading-4" style={{ color: XIRA_QUYUQ }}>
            {son(d.group.usersCount)} ta
          </span>
        )
      }
      tana="px-[18px] pb-[12px] pt-[6px]"
    >
      {qatorlar.length === 0 ? (
        <Bosh>Foydalanuvchi aniqlanmagan — xatolik kirishdan oldin yoki fon jarayonida yuz bergan.</Bosh>
      ) : (
        qatorlar.map((u, i) => (
          <div
            key={`${u.id ?? u.label}-${i}`}
            className="flex h-[48px] items-center gap-[10px]"
            style={
              i === qatorlar.length - 1 && nomalum === 0
                ? undefined
                : { boxShadow: `inset 0 -1px 0 0 ${HOSHIYA}` }
            }
          >
            <Avatar nomi={u.label} olcham={30} />
            <div className="flex min-w-0 flex-1 flex-col gap-[2px]">
              <p className="truncate text-[12.5px] font-medium leading-[17px]" style={{ color: IK }}>
                {u.label}
              </p>
              {/* Telefon SERVERDA niqoblanadi va niqobni ochadigan endpoint
                  panelda yo'q — bu ataylab. */}
              <p className="truncate text-[11.5px] leading-[15px]" style={{ color: OCH_KUL }}>
                {u.sub || YOQ}
              </p>
            </div>
            <span
              className="w-[56px] shrink-0 text-right text-[12px] font-medium leading-4"
              style={{ color: OCH_KUL }}
            >
              {son(u.count)} marta
            </span>
          </div>
        ))
      )}
      {nomalum > 0 && (
        <p className="pt-[10px] text-[11.5px] leading-4" style={{ color: XIRA_QUYUQ }}>
          Yana {son(nomalum)} ta foydalanuvchi faqat noyob belgi bo'yicha sanalgan — ismi
          saqlanmagan.
        </p>
      )}

      {/* ── Niqob haqidagi izoh (Figma 3.12.3 · I · ichki karta) ──────
          Qatorlardagi "+998 90 ••• •• 42" ni admin O'ZI shunday deb
          o'ylamasligi kerak: nuqtalar yuklanmagan ma'lumot emas, ATAYLAB
          qo'yilgan niqob. Izohsiz birinchi savol "to'lig'ini qayerdan
          ko'raman" bo'lardi.

          Figma bu yerga "Ochish" tugmasini ham chizgan, lekin u YO'Q va
          bo'lmaydi ham: telefon xatoliklar jurnaliga umuman yozilmaydi
          (`errlog.Text` uni yozishdan oldin niqoblaydi), bu yerdagi
          qiymat esa o'qish paytida foydalanuvchi yozuvidan olinib
          `admin.maskPhone` orqali niqoblanadi. Xom raqamni qaytaradigan
          endpoint yo'q, ya'ni tugma faqat ishlamaydigan va'da bo'lardi.
          Shuning uchun izoh HAQIQATNI aytadi, tugmani emas.

          Ro'yxat bo'sh bo'lsa izoh ham chiqmaydi: niqoblanadigan telefon
          yo'q joyda u shunchaki shovqin. */}
      {qatorlar.length > 0 && (
        <div className="pt-[12px]">
          <Izohcha ikon={<Lock size={14} aria-hidden />}>
            Telefon va IP har doim niqoblangan holda saqlanadi — xom raqam xatoliklar jurnaliga
            umuman yozilmaydi. Shu sababli niqobni ochadigan tugma ham yo'q: uni ko'rsatadigan
            manba mavjud emas.
          </Izohcha>
        </div>
      )}
    </Karta>
  );
}

/* ══ 9 · Xatolikdan oldingi qadamlar ════════════════════════════════
   Figma 3.12.3 · I bo'limi. Stack trace "QAYERDA yiqildi" degan savolga
   javob beradi, qadamlar esa "QANDAY yetib kelindi" degan savolga —
   xatolikni QAYTA TAKRORLASH uchun aynan ikkinchisi kerak.

   Ma'lumot API'da avvaldan bor edi (`XatoNamuna.steps`), lekin uni
   ko'rsatadigan karta yo'q edi: qadamlar faqat AI kontekst matnining
   ichida ko'rinardi, ya'ni admin ularni o'qish uchun eksport oynasini
   ochib, tayyor promptni ko'zdan kechirishga majbur edi. */

/**
 * Qadam turining rangi va o'zbekcha yorlig'i.
 *
 * Rang TURKUM bo'yicha, "yaxshi/yomon" bo'yicha emas — shuning uchun 500
 * qaytargan so'rov ham kulrang qoladi: kodni matnning o'zi aytadi.
 *
 * · `nav` va `screen` bitta ko'kda: ikkalasi ham "foydalanuvchi QAYERDA
 *   edi" degan savolga javob beradi, ularni ikki rangga ajratish ro'yxatni
 *   chalarang qilardi.
 * · `action` — siyoh: bu yagona turkum, unda odam BIROR NARSA QILDI.
 *   Takrorlash ssenariysi aynan shu qatorlardan yoziladi.
 * · `request` va `response` — kulrang: ular ko'p va ketma-ket keladi,
 *   ko'zni birinchi bo'lib o'ziga tortmasligi kerak.
 * · `crash` — qizil va qalin: u har doim OXIRGI qator va butun ro'yxatning
 *   yakuni.
 *
 * Kalitlar serverdagi `errlog.stepKinds` bilan bir xil: notanish tur
 * `cleanSteps` da `action` ga aylantiriladi, ya'ni bu yerga katalogda
 * yo'q qiymat tushmaydi.
 */
const QADAM: Record<XatoQadam["kind"], { nomi: string; rang: string }> = {
  nav: { nomi: "navigatsiya", rang: KO_K },
  screen: { nomi: "ekran", rang: KO_K },
  action: { nomi: "amal", rang: SIYOH },
  request: { nomi: "so'rov", rang: OCH_KUL },
  response: { nomi: "javob", rang: OCH_KUL },
  crash: { nomi: "yiqilish", rang: QIZIL },
};

export function QadamlarKarta({ d, hozir }: { d: AdminErrorDetail; hozir: number }) {
  const qadamlar = d.sample?.steps ?? [];

  return (
    <Karta
      sarlavha={
        <span className="flex min-w-0 items-center gap-[7px]">
          <Footprints size={14} color={OCH_KUL} className="shrink-0" aria-hidden />
          <span className="truncate">Xatolikdan oldingi qadamlar</span>
        </span>
      }
      amal={
        qadamlar.length > 0 ? (
          // Server oxirgi 20 tasini saqlaydi (`errlog.maxStepCount`), shuning
          // uchun son "hammasi shu" degani emas — "oxirgi" so'zi ataylab.
          <span className="shrink-0 text-[12px] font-medium leading-4" style={{ color: XIRA_QUYUQ }}>
            oxirgi {son(qadamlar.length)} ta
          </span>
        ) : undefined
      }
      tana="px-[18px] pb-[14px] pt-[6px]"
    >
      {qadamlar.length === 0 ? (
        // Bu bo'sh holat vaqtinchalik nosozlik EMAS, tizimning hozirgi
        // haqiqati: `/api/client-errors` endpoint'i tayyor, lekin hech bir
        // mijoz ilovasi hali breadcrumb yubormaydi. "Aniqlanmagan" deb
        // yozsak, admin yo'q nosozlikni qidirib ketardi.
        <Bosh>
          Qadamlar yig'ilmagan — mijoz ilovasi hali breadcrumb yubormaydi. Ular
          /api/client-errors hisoboti bilan kela boshlagach, shu yerda o'zi paydo bo'ladi.
        </Bosh>
      ) : (
        <div className="flex min-w-0 flex-col gap-[12px]">
          <div className="flex min-w-0 flex-col">
            {qadamlar.map((q, i) => {
              const t = new Date(q.at);
              const yaroqli = !Number.isNaN(t.getTime());
              const k = QADAM[q.kind];
              const oxirgi = i === qadamlar.length - 1;
              const yiqildi = q.kind === "crash";
              return (
                <div
                  key={`${q.at}-${i}`}
                  className="flex min-w-0 flex-col gap-[3px] py-[8px]"
                  style={oxirgi ? undefined : { boxShadow: `inset 0 -1px 0 0 ${HOSHIYA}` }}
                >
                  {/* Vaqt · nuqta · tur — bitta satrda, matn esa ostida.
                      Karta 392 px ustunda turadi: to'rttasini bir qatorga
                      tersak, matn 120 px ga siqilib o'qilmay qolardi. */}
                  <div className="flex min-w-0 items-center gap-[7px]">
                    <span
                      className="shrink-0 text-[11.5px] font-medium leading-4 tabular-nums"
                      style={{ color: XIRA_QUYUQ }}
                      title={yaroqli ? kunSoat(t, hozir) : undefined}
                    >
                      {yaroqli ? soatSek(t) : YOQ}
                    </span>
                    <span
                      aria-hidden
                      className="h-[5px] w-[5px] shrink-0 rounded-full"
                      style={{ background: k.rang }}
                    />
                    <span
                      className="truncate text-[11.5px] font-medium leading-4"
                      style={{ color: k.rang }}
                    >
                      {k.nomi}
                    </span>
                  </div>
                  <p
                    className={`break-words text-[12.5px] leading-[17px] ${yiqildi ? "font-semibold" : "font-medium"}`}
                    style={{ color: q.text ? (yiqildi ? QIZIL : KUL) : XIRA_QUYUQ }}
                  >
                    {q.text || YOQ}
                  </p>
                </div>
              );
            })}
          </div>

          {/* Qadamlar mijozdan keladi, ya'ni ISHONCHSIZ manba. Ikki
              cheklovni ochiq aytamiz: ro'yxat qisqartirilgan (admin
              "boshidan beri hammasi shu" deb o'ylamasin) va matn
              niqoblangan (`***` ko'rgan admin uni buzuq ma'lumot deb
              hisoblamasin). */}
          <Izohcha>
            Server har bir namunada oxirgi 20 ta qadamni saqlaydi. Matn yozishdan oldin
            niqoblanadi — token, OTP, telefon va IP qadamlar ichiga tushmaydi.
          </Izohcha>
        </div>
      )}
    </Karta>
  );
}

/* ══ 10 · Ta'sir taqsimoti ══════════════════════════════════════════
   Figma 3.12.3 · K bo'limi. "Muhit" kartasi ODDIY namunani ko'rsatadi
   (bitta hodisa qanday qurilmada bo'lgani), bu esa BUTUN guruhni: xatolik
   hamma joyda chiqyaptimi yoki bitta Xiaomi'da, eski versiyadami yoki
   oxirgisidami. Tuzatishni boshlashdan oldingi eng birinchi savol shu. */

/**
 * Ro'yxatga sig'magan qiymatlarning QOLDIQ qatori — backend o'zi qo'shadi.
 *
 * Yangi javoblarda qator `other: true` bayrog'i bilan keladi va shu nom
 * KERAK EMAS. U faqat bayroqsiz eski javoblar uchun zaxira: o'sha
 * yozuvlarda qoldiqni nomidan boshqa hech narsa ajratib turmaydi.
 */
const QOLDIQ = "Boshqa";

/**
 * Ulush rangi — O'RIN bo'yicha, foiz bo'yicha emas.
 *
 * Sabab: taqsimotning ma'nosi nisbatda. 30 % lik ulush oltita qurilma
 * orasida birinchi bo'lsa — bu "asosiy aybdor", ikkitasi orasida esa
 * "ozchilik". Shuning uchun birinchi qator doim qizil, ikkinchisi to'q
 * sariq, qolganlari ko'k — lekin 8 % dan pastlari kulrang: ular shovqin
 * va ko'zni birinchi ikkitasidan chalg'itmasligi kerak.
 *
 * "Boshqa" esa O'RNIDAN QAT'I NAZAR kulrang: u bitta qiymat emas, bir
 * necha mayda qiymatning yig'indisi. Qoldiq birinchi o'ringa chiqib
 * qolsa (ko'p xilma-xil brend), qizil bo'yash "asosiy aybdor — Boshqa"
 * degan ma'nosiz xulosa berardi, ustiga bosib filtrlash ham mumkin emas.
 *
 * Qoldiq `other` BAYROG'I bo'yicha aniqlanadi: yorliq matni o'zgarsa
 * (tarjima, qayta nomlash) nom bo'yicha tekshiruv jimgina buzilib,
 * qoldiq qatori qizil bo'lib qolardi. `QOLDIQ` nomi faqat bayroqsiz
 * ESKI javoblar uchun zaxira sifatida qoladi.
 */
function ulushRang(u: XatoUlush, orin: number): string {
  if (u.other || (u.key ?? "").trim() === QOLDIQ) return HOSHIYA_QUYUQ;
  if (orin === 0) return QIZIL;
  if (orin === 1) return ORANJ;
  return u.pct >= 8 ? KO_K : HOSHIYA_QUYUQ;
}

function TasirUstun({ nomi, qatorlar }: { nomi: string; qatorlar: XatoUlush[] }) {
  return (
    <div className="flex min-w-0 flex-col gap-[10px]">
      <div className="text-[12px] font-semibold leading-4" style={{ color: IK }}>
        {nomi}
      </div>
      {qatorlar.length === 0 ? (
        <p className="text-[11.5px] leading-4" style={{ color: XIRA_QUYUQ }}>
          Ma'lumot yig'ilmagan.
        </p>
      ) : (
        <div className="flex min-w-0 flex-col gap-[9px]">
          {qatorlar.map((u, i) => (
            <div key={`${u.key}-${i}`} className="flex min-w-0 flex-col gap-[5px]">
              <div className="flex min-w-0 items-baseline justify-between gap-[8px]">
                <span className="truncate text-[12px] leading-[16px]" style={{ color: KUL }}>
                  {u.key || YOQ}
                </span>
                <span
                  className="shrink-0 text-[11.5px] font-medium leading-4"
                  style={{ color: OCH_KUL }}
                >
                  {u.pct}% · {son(u.n)} hodisa
                </span>
              </div>
              {/* Zolak 100 % kenglikda turadi va ichidagi bo'lak foizni
                  ko'rsatadi. 0 % qatorlar ham bor (yaxlitlash) — ularga
                  eng kam 2 % beriladi, aks holda qator bo'sh ko'rinardi. */}
              <div className="h-[6px] w-full rounded-full" style={{ background: HOSHIYA }}>
                <div
                  className="h-[6px] rounded-full"
                  style={{
                    width: `${Math.min(100, Math.max(2, u.pct))}%`,
                    background: ulushRang(u, i),
                  }}
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function TasirKarta({ d }: { d: AdminErrorDetail }) {
  const t = d.impact;
  const brand = t?.brand ?? [];
  const os = t?.os ?? [];
  const app = t?.app ?? [];
  const bosh = brand.length === 0 && os.length === 0 && app.length === 0;

  return (
    <Karta
      sarlavha="Ta'sir taqsimoti"
      amal={
        <span className="shrink-0 text-[12px] font-medium leading-4" style={{ color: XIRA_QUYUQ }}>
          Oxirgi 30 kun
        </span>
      }
      tana="p-[18px]"
    >
      {bosh ? (
        <Bosh>
          Qurilma ma'lumoti bilan kelgan hodisa yo'q — xatolik server tomonda yoki eski
          versiyadan tushgan.
        </Bosh>
      ) : (
        <div className="flex min-w-0 flex-col gap-[14px]">
          <div className="grid min-w-0 gap-x-[22px] gap-y-[16px] md:grid-cols-3">
            <TasirUstun nomi="Qurilma brendi" qatorlar={brand} />
            {/* Sarlavha "Operatsion tizim" emas: ustunda `Android 14`,
                `iOS 17` turadi, ya'ni tizim NOMI emas — VERSIYASI. Eski
                nom bilan admin "Android" degan bitta qator kutardi. */}
            <TasirUstun nomi="OS versiyasi" qatorlar={os} />
            <TasirUstun nomi="Ilova versiyasi" qatorlar={app} />
          </div>
          {/* Foizlar BUTUN kesim bo'yicha: backend ro'yxatga sig'magan
              mayda qiymatlarni «Boshqa» qatoriga yig'adi, shuning uchun
              ustun jami 100 % ni beradi (yaxlitlashdan ±1 %). Avvalgi
              "foizlar 100 ga yetmasligi mumkin" yozuvi endi yolg'on va
              uni o'qigan admin haqiqiy jamini kam deb baholardi. */}
          <p className="text-[11.5px] leading-4" style={{ color: XIRA_QUYUQ }}>
            Foizlar butun kesim bo'yicha hisoblangan: ro'yxatga sig'magan qiymatlar «Boshqa»
            qatoriga yig'ilgan. Qurilmasi noma'lum hodisalar hisobga olinmagan.
          </p>
        </div>
      )}
    </Karta>
  );
}
