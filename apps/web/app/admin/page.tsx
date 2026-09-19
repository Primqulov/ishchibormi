"use client";
import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowUp } from "lucide-react";
import { api, DashboardStats, AdminStats, DayPoint, NameCount, platformLabel } from "@/lib/api";

/* ─────────────────────────────────────────────────────────────────────
   Figma: "1.2 / 3.2 · Desktop karkas + Dashboard — superadmin",
          "3.2a · Dashboard holatlari",
          "3.2b · Grafik paneli — yangilangan dizayn",
          "Animatsiya · vaqt chizig'i (jami 3.2 soniya)".

   Admin paneli o'z palitrasida chizilgan (ko'k #004ac6, sayt brendi
   #0038d8 emas), shuning uchun ranglar kirish sahifasidagi kabi
   to'g'ridan-to'g'ri yozilgan. Figma'dagi INSIDE hoshiya CSS border emas —
   u `inset` box-shadow bilan beriladi, aks holda quti 2 px kattarib ketadi.
   ───────────────────────────────────────────────────────────────────── */
const KO_K = "#004ac6";
const IK = "#0b1c30";
const KUL = "#434655";
const OCH_KUL = "#737686";
const XIRA = "#a7acb9";
const HOSHIYA = "#eaecf2";
const HOSHIYA_QUYUQ = "#c3c6d7";
const YASHIL = "#1fa463";
const QIZIL = "#e5484d";
const ORANJ = "#e8890c";
const TREK = "#e7eaf3";
const TUR = "#eaedf5"; // grafikdagi uzuq to'r chizig'i
const PANEL_SOYA = "0 2px 8px rgba(11, 28, 48, 0.06)";

const DAVRLAR = [7, 30, 90] as const;
type Davr = (typeof DAVRLAR)[number];

// Grafik maydonining o'lchamlari (Figma 3.2b).
const CH_BALAND = 112; // qiymat maydoni
const CH_OQ = 26; // y o'qi yorlig'i ustuni
const CH_TIRQISH = 8; // y o'qi bilan maydon orasi
const CH_CHET = 6; // birinchi/oxirgi nuqta uchun ichki chekinish
const CH_DELTA = 24; // "kunlik o'zgarish" chizig'ining balandligi

const OY_QISQA = ["yan", "fev", "mar", "apr", "may", "iyun", "iyul", "avg", "sen", "okt", "noy", "dek"];
const OY_TOLIQ = [
  "yanvar", "fevral", "mart", "aprel", "may", "iyun",
  "iyul", "avgust", "sentabr", "oktabr", "noyabr", "dekabr",
];
const HAFTA_QISQA = ["Ya", "Du", "Se", "Cho", "Pa", "Ju", "Sha"]; // getDay(): 0 = yakshanba
const HAFTA_TOLIQ = ["yakshanba", "dushanba", "seshanba", "chorshanba", "payshanba", "juma", "shanba"];

/* ── Kichik yordamchilar ─────────────────────────────────────────────
   Barcha qiymatlar backenddan keladi, shuning uchun har biri son ekanligi
   tekshiriladi: kutilmagan javob grafikni NaN yoki Infinity bilan
   buzib qo'ymasligi kerak. */
const raqam = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) ? v : 0);

function son(v: number): string {
  if (!Number.isFinite(v)) return "—";
  const t = Math.round(Math.abs(v)).toString().replace(/\B(?=(\d{3})+(?!\d))/g, " ");
  return (v < 0 ? "-" : "") + t;
}
const belgili = (v: number): string => (v > 0 ? "+" : "") + son(v);
const foiz = (v: number): string => (v > 0 ? "+" : v < 0 ? "-" : "") + Math.abs(v).toFixed(1) + "%";

// "YYYY-MM-DD" ni mahalliy sana sifatida o'qiydi. new Date(iso) UTC yarim
// tunini beradi va manfiy vaqt mintaqalarida kunni bir kunga surib yuboradi.
function kun(iso: unknown): Date {
  const p = String(iso ?? "").split("-");
  const y = Number(p[0]), m = Number(p[1]), d = Number(p[2]);
  if (!Number.isFinite(y) || !Number.isFinite(m) || !Number.isFinite(d)) return new Date(NaN);
  return new Date(y, m - 1, d);
}
const yaroqli = (d: Date) => !Number.isNaN(d.getTime());
function sanaQisqa(d: Date, davr: Davr): string {
  if (!yaroqli(d)) return "—";
  return davr === 7 ? `${HAFTA_QISQA[d.getDay()]} ${d.getDate()}` : `${d.getDate()}-${OY_QISQA[d.getMonth()]}`;
}
function sanaToliq(d: Date): string {
  if (!yaroqli(d)) return "—";
  return `${d.getDate()}-${OY_TOLIQ[d.getMonth()]}, ${HAFTA_TOLIQ[d.getDay()]}`;
}

// Y o'qi bo'linmasi. Figma'dagi beshta grafikning hammasi shu qoidaga
// tushadi: bo'linma {1,2,4,8}×10^k dan max/3 dan katta bo'lgan eng kichigi.
function nishonQadami(eng: number): number {
  const xom = Math.max(1, eng) / 3;
  if (!Number.isFinite(xom) || xom <= 0) return 1;
  const daraja = Math.floor(Math.log10(xom));
  for (let e = daraja; e <= daraja + 3; e++) {
    for (const m of [1, 2, 4, 8]) {
      const q = m * Math.pow(10, e);
      if (q >= xom) return Math.max(1, Math.round(q));
    }
  }
  return 1;
}

interface Nuqta { d: Date; v: number }

// Ustunli diagramma qatori. Ba'zi panellarda ustun rangi qatorning ma'nosini
// bildiradi (ariza holati, platforma) — qolganlarida hamma ustun ko'k.
interface Ustun extends NameCount { rang?: string }

// Platforma kodi → ustun rangi (Figma 1.2/3.2). Kodlar backenddan keladi,
// shuning uchun noma'lum qiymat kulrangga tushadi.
const PLATFORMA_RANGI: Record<string, string> = {
  web: KO_K,
  android: YASHIL,
  ios: IK,
  telegram: ORANJ,
  unknown: OCH_KUL,
};

// Kutilmagan uzun javob sahifani qotirib qo'ymasligi uchun uzunlik cheklanadi
// (backend eng ko'pi bilan 90 nuqta yuboradi).
function nuqtalarga(rows: DayPoint[] | undefined): Nuqta[] {
  if (!Array.isArray(rows)) return [];
  return rows.slice(0, 400).map((r) => ({ d: kun(r?.date), v: raqam(r?.count) }));
}

// Konteyner kengligini kuzatadi. Grafik koordinatalari haqiqiy piksellarda
// hisoblanadi — shunda chiziq qalinligi va nuqtalar cho'zilib ketmaydi.
// Callback-ref ishlatilgan: element panel "yuklanmoqda" holatidan keyin
// paydo bo'ladi, oddiy useRef bilan effekt o'sha paytda qayta ishga
// tushmasdi va kenglik 0 bo'lib qolardi.
function useKenglik<T extends HTMLElement>() {
  const [el, setEl] = useState<T | null>(null);
  const [w, setW] = useState(0);
  useEffect(() => {
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((e) => {
      const r = e[0]?.contentRect;
      if (r) setW(Math.max(0, Math.round(r.width)));
    });
    ro.observe(el);
    setW(Math.max(0, Math.round(el.getBoundingClientRect().width)));
    return () => ro.disconnect();
  }, [el]);
  return { ref: setEl, w };
}

/* ── Sahifa ──────────────────────────────────────────────────────── */
export default function AdminDashboard() {
  // undefined = hali kelmadi, null = kelmadi (tarmoq/server xatosi).
  const [kpi, setKpi] = useState<DashboardStats | null | undefined>(undefined);
  const [stats, setStats] = useState<AdminStats | null | undefined>(undefined);
  const [davr, setDavr] = useState<Davr>(30);
  // Kirish animatsiyasi 3.2 s da tugaydi. Undan keyin davr almashtirilsa,
  // grafik kutib turmasdan darhol qayta chiziladi.
  const [ochilish, setOchilish] = useState(true);

  useEffect(() => {
    const t = setTimeout(() => setOchilish(false), 3300);
    return () => clearTimeout(t);
  }, []);

  useEffect(() => {
    let bekor = false;
    api
      .get<DashboardStats>("/api/admin/dashboard", { auth: "admin" } as any)
      .then((r) => { if (!bekor) setKpi(r); })
      .catch(() => { if (!bekor) setKpi(null); });
    return () => { bekor = true; };
  }, []);

  useEffect(() => {
    let bekor = false;
    setStats(undefined);
    // Davr faqat 7/30/90 bo'la oladi (DAVRLAR dan tanlanadi), shuning uchun
    // so'rovga foydalanuvchi kiritgan matn tushmaydi.
    api
      .get<AdminStats>(`/api/admin/stats?days=${davr}`, { auth: "admin" } as any)
      .then((r) => { if (!bekor) setStats(r); })
      .catch(() => { if (!bekor) setStats(null); });
    // Eskirgan javob yangisining ustiga yozilmasin: davr almashsa, avvalgi
    // so'rovning natijasi e'tiborsiz qoldiriladi.
    return () => { bekor = true; };
  }, [davr]);

  const yuklanmoqda = kpi === undefined;
  const funnel = stats?.funnel || {};
  // Voronkada ustun rangi ariza holatini bildiradi (Figma 1.2/3.2).
  //
  // Ranglar ATAYLAB shu ekranning o'z Figma kadridan (3.2) olingan va
  // arizalar ro'yxatidagidan (3.6) farq qiladi: u yerda "qabul qilingan"
  // ko'k, "bajarilgan" yashil. Ikkisini tenglashtirish 3.2 kadrini
  // qayta chizishni talab qiladi — bu esa boshqa ekranning ishi.
  const voronka: Ustun[] | null | undefined = stats
    ? [
        { name: "Yuborilgan", count: raqam(funnel["pending"]), rang: ORANJ },
        { name: "Qabul qilingan", count: raqam(funnel["accepted"]), rang: YASHIL },
        { name: "Rad etilgan", count: raqam(funnel["rejected"]), rang: QIZIL },
        { name: "Bekor qilingan", count: raqam(funnel["cancelled"]), rang: OCH_KUL },
        { name: "Bajarilgan", count: raqam(funnel["completed"]), rang: KO_K },
      ]
    : stats === null
      ? null
      : undefined;

  // Vaqt chizig'i: KPI kartalar 0.55 s → 1.60 s, 11 ta karta, 0.055 s farq.
  let k = 0;
  const kpiKechikish = () => 0.55 + (k++) * 0.055;

  const faolOyna = raqam(stats?.platforms?.activeWindowDays) || 30;
  const panellar: {
    sarlavha: string;
    izoh?: string;
    tur: "grafik" | "ustun";
    rows?: Ustun[] | null;
    seriya?: DayPoint[];
    birlik?: string;
  }[] = [
    { sarlavha: "Foydalanuvchi o'sishi", tur: "grafik", seriya: stats?.userGrowth, birlik: "yangi foydalanuvchi" },
    { sarlavha: "Yangi e'lonlar", tur: "grafik", seriya: stats?.elonGrowth, birlik: "yangi e'lon" },
    { sarlavha: "Arizalar voronkasi", tur: "ustun", rows: voronka },
    { sarlavha: "Eng ommabop turkumlar", tur: "ustun", rows: qatorlar(stats?.topCategories, stats) },
    { sarlavha: "Viloyatlar bo'yicha foydalanuvchilar", tur: "ustun", rows: qatorlar(stats?.regions, stats) },
    { sarlavha: "Qayerdan ro'yxatdan o'tishgan", tur: "ustun", rows: platformaQatorlari(stats?.platforms?.signup, stats) },
    {
      sarlavha: `Faol foydalanuvchilar (oxirgi ${faolOyna} kun)`,
      izoh: `Oxirgi ${faolOyna} kunda kamida bir marta so'rov yuborganlar`,
      tur: "ustun",
      rows: platformaQatorlari(stats?.platforms?.active, stats),
    },
  ];
  // Vaqt chizig'i: chiziqli grafiklar 1.98 s dan, ustunli diagrammalar
  // 1.94 s dan boshlanadi — shuning uchun ikkalasi alohida sanaladi.
  let grafikIndeks = 0;
  let ustunIndeks = 0;

  return (
    <div className="grid gap-4" style={{ color: IK }}>
      {/* ── KPI kartalar (11 ta) ───────────────────────────────── */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <Karta label="Jami foydalanuvchi" value={kpi?.users} loading={yuklanmoqda} delay={kpiKechikish()} />
        <Karta label="Faol" value={kpi?.activeUsers} loading={yuklanmoqda} delay={kpiKechikish()} />
        <Karta label="Bloklangan" value={kpi?.blockedUsers} loading={yuklanmoqda} delay={kpiKechikish()} tone="danger" />
        <Karta label="Jami e'lon" value={kpi?.elons} loading={yuklanmoqda} delay={kpiKechikish()} />
        <Karta label="Faol e'lon (rec.)" value={kpi?.recruitingElons} loading={yuklanmoqda} delay={kpiKechikish()} />
        <Karta label="To'lgan e'lon" value={kpi?.filledElons} loading={yuklanmoqda} delay={kpiKechikish()} />
        <Karta label="Bajarilgan ish" value={kpi?.completed} loading={yuklanmoqda} delay={kpiKechikish()} tone="success" />
        <Karta label="Bugungi yangi user" value={kpi?.todayUsers} loading={yuklanmoqda} delay={kpiKechikish()} tone="brand" />
        <Karta label="Bugungi yangi e'lon" value={kpi?.todayElons} loading={yuklanmoqda} delay={kpiKechikish()} tone="brand" />
        <Karta label="Ochiq shikoyat" value={kpi?.openReports} loading={yuklanmoqda} delay={kpiKechikish()} tone="danger" />
        <Karta label="Ochiq murojaat" value={kpi?.openFeedback} loading={yuklanmoqda} delay={kpiKechikish()} tone="danger" />
      </div>

      {/* ── Platforma — foydalanuvchilar qaysi klientdan foydalanmoqda.
             Alohida blok, KPI setiga qo'shib yuborilmagan: bu boshqa savol
             va yonma-yon turgan to'rtta karta yig'indisi jami foydalanuvchiga
             teng bo'lishi ko'rinib turishi kerak. */}
      <div className="grid gap-2">
        <div className="ib-anim ib-anim-fade text-sm font-semibold" style={{ animationDelay: "1.10s" }}>
          Foydalanuvchilar qaysi platformada
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Karta label="Veb" value={kpi?.webUsers} loading={yuklanmoqda} delay={1.15} anim="plat" tone="brand" />
          <Karta label="Android" value={kpi?.androidUsers} loading={yuklanmoqda} delay={1.23} anim="plat" tone="success" />
          <Karta label="iOS" value={kpi?.iosUsers} loading={yuklanmoqda} delay={1.31} anim="plat" />
          <Karta label="Telegram" value={kpi?.telegramUsers} loading={yuklanmoqda} delay={1.39} anim="plat" />
          <Karta label="Noma'lum" value={kpi?.unknownPlatformUsers} loading={yuklanmoqda} delay={1.47} anim="plat" />
        </div>
        <p className="ib-anim ib-anim-fade text-xs" style={{ color: OCH_KUL, animationDelay: "1.10s" }}>
          Oxirgi ishlatilgan klient bo&apos;yicha. &quot;Noma&apos;lum&quot; — bu hisob
          yuritilishidan oldin ro&apos;yxatdan o&apos;tganlar va ilovaning eski versiyalari.
        </p>
      </div>

      {/* ── O'sish davri almashtirgichi (3.2a) ─────────────────── */}
      <div className="ib-anim ib-anim-fade flex items-center gap-2" style={{ animationDelay: "1.50s" }}>
        <span className="text-[13px]" style={{ color: KUL }}>O&apos;sish davri:</span>
        {DAVRLAR.map((d) => {
          const faol = davr === d;
          return (
            <button
              key={d}
              type="button"
              onClick={() => setDavr(d)}
              aria-pressed={faol}
              className={`h-8 w-20 rounded-lg text-[13px] transition-colors ${faol ? "font-semibold text-white" : "bg-white font-medium hover:bg-[#f4f6fc]"}`}
              style={{
                background: faol ? KO_K : undefined,
                color: faol ? undefined : IK,
                boxShadow: faol ? undefined : `inset 0 0 0 1px ${HOSHIYA_QUYUQ}`,
              }}
            >
              {d} kun
            </button>
          );
        })}
      </div>

      {/* ── Panellar ───────────────────────────────────────────── */}
      <div className="grid gap-5 lg:grid-cols-2">
        {panellar.map((p, i) => {
          const trend = p.tur === "grafik" ? trendHisobi(stats ? p.seriya : null) : null;
          return (
          <Panel
            key={p.sarlavha}
            sarlavha={p.tur === "grafik" ? `${p.sarlavha} · ${davr} kun` : p.sarlavha}
            izoh={p.izoh}
            ong={trend === null ? undefined : <TrendPil qiymat={trend} />}
            delay={1.575 + i * 0.05}
          >
            {p.tur === "grafik" ? (
              <Grafik
                birlik={p.birlik || ""}
                davr={davr}
                qatorlar={stats === undefined ? undefined : stats === null ? null : p.seriya || []}
                kechikish={ochilish ? 1.98 + (grafikIndeks++) * 0.12 : 0}
              />
            ) : (
              <Ustunlar qatorlar={p.rows} panelIndeks={ustunIndeks++} />
            )}
          </Panel>
          );
        })}
      </div>
    </div>
  );
}

// stats hali kelmagan bo'lsa undefined, kelmagan bo'lsa null qaytaramiz —
// panel shunga qarab "yuklanmoqda" yoki "ma'lumot yo'q" holatini ko'rsatadi.
function qatorlar(rows: NameCount[] | undefined, stats: AdminStats | null | undefined): Ustun[] | null | undefined {
  if (stats === undefined) return undefined;
  if (stats === null) return null;
  return (Array.isArray(rows) ? rows : []).slice(0, 50).map((r) => ({ name: String(r?.name ?? ""), count: raqam(r?.count) }));
}

// platformaQatorlari backend qaytargan kodlarni ("web", "android", …) panelda
// ko'rsatiladigan nomlarga aylantiradi. Tartib va nol qiymatli qatorlar
// backenddan kelgan holicha qoladi — u har doim to'liq ro'yxat yuboradi,
// shunda ustun nolga tushganda grafikdan yo'qolib qolmaydi.
function platformaQatorlari(rows: NameCount[] | undefined, stats: AdminStats | null | undefined): Ustun[] | null | undefined {
  const x = qatorlar(rows, stats);
  return x
    ? x.map((r) => ({ name: platformLabel(r.name), count: r.count, rang: PLATFORMA_RANGI[r.name] || OCH_KUL }))
    : x;
}

/* ── KPI karta ───────────────────────────────────────────────────── */
function Karta({
  label, value, loading, delay, tone, anim = "kpi",
}: {
  label: string;
  value?: number;
  loading: boolean;
  delay: number;
  tone?: "danger" | "success" | "brand";
  anim?: "kpi" | "plat";
}) {
  const rang = tone === "danger" ? QIZIL : tone === "success" ? YASHIL : tone === "brand" ? KO_K : IK;
  return (
    <div
      className={`ib-anim ib-anim-${anim} h-[104px] rounded-[14px] bg-white px-[18px] pt-4`}
      style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${PANEL_SOYA}`, animationDelay: `${delay.toFixed(3)}s` }}
    >
      <div className="text-xs font-medium leading-4" style={{ color: OCH_KUL }}>{label}</div>
      {/* Yorliq darhol chiziladi, son o'rnida chiziqcha turadi — karta joyi
          sakramaydi (Figma 3.2a · "Yuklanmoqda"). */}
      <div className="mt-1.5 flex h-11 items-center">
        {loading ? (
          <span className="block h-1.5 w-8 rounded-full" style={{ background: HOSHIYA_QUYUQ }} role="img" aria-label="yuklanmoqda" />
        ) : (
          <span className="text-[32px] font-bold leading-none" style={{ color: rang }}>
            {typeof value === "number" ? son(value) : "—"}
          </span>
        )}
      </div>
    </div>
  );
}

/* ── Panel qobig'i ───────────────────────────────────────────────── */
function Panel({
  sarlavha, izoh, ong, delay, children,
}: {
  sarlavha: string;
  izoh?: string;
  ong?: React.ReactNode;
  delay: number;
  children: React.ReactNode;
}) {
  return (
    <section
      className="ib-anim ib-anim-panel rounded-[14px] bg-white p-4"
      style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${PANEL_SOYA}`, animationDelay: `${delay.toFixed(3)}s` }}
    >
      {/* Sarlavha qatori: chapda nom (va izoh), o'ngda o'zgarish nishoni. */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold leading-[18px]">{sarlavha}</h2>
          {izoh && <p className="mt-1 text-xs leading-4" style={{ color: OCH_KUL }}>{izoh}</p>}
        </div>
        {ong}
      </div>
      {children}
    </section>
  );
}

/* ── Davr o'zgarishi nishoni ─────────────────────────────────────────
   Figma 3.2b: nishon panel sarlavhasi qatorida turadi. Manfiy bo'lsa
   qizil — chiziq rangi esa o'zgarmaydi (u ma'lumot, holat emas). */
function TrendPil({ qiymat }: { qiymat: number }) {
  const past = qiymat < 0;
  return (
    <span
      className="flex h-6 shrink-0 items-center gap-1 rounded-full px-[10px] text-[12px] font-semibold"
      style={{ background: past ? "#fcf2f2" : "#eff8f4", color: past ? QIZIL : YASHIL }}
      title="Davrning ikkinchi yarmi birinchi yarmiga nisbatan"
    >
      {past ? <ArrowDown size={13} strokeWidth={2.6} aria-hidden /> : <ArrowUp size={13} strokeWidth={2.6} aria-hidden />}
      {foiz(qiymat)}
    </span>
  );
}

// Davr o'zgarishi: oynaning ikkinchi yarmi birinchi yarmiga nisbatan.
// Hisoblab bo'lmasa (ma'lumot yo'q yoki birinchi yarim nol) null qaytadi —
// panel sarlavhasida nishon umuman chiqmaydi.
function trendHisobi(rows: DayPoint[] | null | undefined): number | null {
  if (!rows) return null;
  const p = nuqtalarga(rows);
  const n = p.length;
  const yarim = Math.floor(n / 2);
  if (n < 2 || yarim === 0) return null;
  const bJami = p.slice(0, yarim).reduce((s, x) => s + x.v, 0);
  const iJami = p.slice(n - yarim).reduce((s, x) => s + x.v, 0);
  if (bJami <= 0) return null;
  return ((iJami - bJami) / bJami) * 100;
}

// Figma 3.2a · "Bo'sh panel": uzuq ramkali quti, markazda izoh.
function BoshQuti({ matn }: { matn: string }) {
  return (
    <div
      className="mt-[10px] grid h-[86px] place-items-center rounded-[10px] text-[13px]"
      style={{ border: `1px dashed ${HOSHIYA_QUYUQ}`, color: OCH_KUL }}
    >
      {matn}
    </div>
  );
}

/* ── Ustunli diagramma ───────────────────────────────────────────── */
function Ustunlar({ qatorlar: rows, panelIndeks }: { qatorlar: Ustun[] | null | undefined; panelIndeks: number }) {
  if (rows === undefined) return <BoshQuti matn="Yuklanmoqda…" />;
  if (rows === null) return <BoshQuti matn="Ma'lumot yuklanmadi" />;
  const jami = rows.reduce((s, r) => s + r.count, 0);
  if (!rows.length || jami === 0) return <BoshQuti matn="Ma'lumot yo'q" />;
  const eng = Math.max(1, ...rows.map((r) => r.count));
  return (
    <div className="mt-[10px] flex flex-col gap-[9px]">
      {rows.map((r, i) => (
        // Kalitga indeks ham qo'shilgan: backend bir xil nomni ikki marta
        // qaytarib qolsa, React kalitlari to'qnashmasin.
        <div key={`${r.name}-${i}`} className="grid h-[17px] grid-cols-[105px_1fr_41px] items-center gap-[10px] max-[520px]:grid-cols-[80px_1fr_41px]">
          <div className="truncate text-xs" style={{ color: KUL }} title={r.name}>{r.name || "—"}</div>
          <div className="h-2 overflow-hidden rounded-full" style={{ background: TREK }}>
            <div
              className="ib-anim ib-anim-bar h-full rounded-full"
              style={{
                width: `${Math.max(0, Math.min(100, (r.count / eng) * 100)).toFixed(2)}%`,
                background: r.rang || KO_K,
                // Vaqt chizig'i: ustunli diagrammalar 1.94 s → 2.69 s.
                animationDelay: `${(1.94 + panelIndeks * 0.055 + i * 0.025).toFixed(3)}s`,
              }}
            />
          </div>
          <div className="text-right text-xs font-semibold tabular-nums">{son(r.count)}</div>
        </div>
      ))}
    </div>
  );
}

/* ── O'sish grafigi (Figma 3.2b) ─────────────────────────────────── */
function Grafik({
  birlik, davr, qatorlar: rows, kechikish,
}: {
  birlik: string;
  davr: Davr;
  qatorlar: DayPoint[] | null | undefined;
  kechikish: number;
}) {
  const { ref, w } = useKenglik<HTMLDivElement>();
  const [ustida, setUstida] = useState<number | null>(null);

  const nuqtalar = useMemo(() => nuqtalarga(rows || undefined), [rows]);
  const n = nuqtalar.length;
  const jami = useMemo(() => nuqtalar.reduce((s, p) => s + p.v, 0), [nuqtalar]);

  // Kunlik o'zgarish: i-nuqta bilan undan oldingisi orasidagi farq.
  const farqlar = useMemo(() => nuqtalar.map((p, i) => (i === 0 ? 0 : p.v - nuqtalar[i - 1].v)), [nuqtalar]);

  const olcham = useMemo(() => {
    const eng = n ? Math.max(0, ...nuqtalar.map((p) => p.v)) : 0;
    const qadam = nishonQadami(eng);
    const bolim = Math.max(2, Math.ceil(eng / qadam) || 2);
    return { qadam, bolim, cho: qadam * bolim };
  }, [nuqtalar, n]);

  if (rows === undefined) return <BoshQuti matn="Yuklanmoqda…" />;
  if (rows === null) return <BoshQuti matn="Ma'lumot yuklanmadi" />;
  if (!n || jami === 0) return <BoshQuti matn="Ma'lumot yo'q" />;

  const ortacha = Math.round(jami / n);

  // Eng katta o'sish / pasayish kuni.
  let osishI = -1, pasayishI = -1;
  farqlar.forEach((f, i) => {
    if (i === 0) return;
    if (f > 0 && (osishI < 0 || f > farqlar[osishI])) osishI = i;
    if (f < 0 && (pasayishI < 0 || f < farqlar[pasayishI])) pasayishI = i;
  });

  // Cho'qqi va eng past nuqta.
  let choqqiI = 0, pastI = 0;
  nuqtalar.forEach((p, i) => {
    if (p.v > nuqtalar[choqqiI].v) choqqiI = i;
    if (p.v < nuqtalar[pastI].v) pastI = i;
  });
  const belgiBor = n > 1 && nuqtalar[choqqiI].v !== nuqtalar[pastI].v;

  const W = Math.max(0, w);
  const ichki = Math.max(0, W - CH_CHET * 2);
  const X = (i: number) => (n <= 1 ? W / 2 : CH_CHET + (i * ichki) / (n - 1));
  const Y = (v: number) => CH_BALAND - (Math.max(0, Math.min(olcham.cho, v)) / olcham.cho) * CH_BALAND;

  const chiziq = nuqtalar.map((p, i) => `${i ? "L" : "M"}${X(i).toFixed(1)},${Y(p.v).toFixed(1)}`).join(" ");
  const soya = `${chiziq} L${X(n - 1).toFixed(1)},${CH_BALAND} L${X(0).toFixed(1)},${CH_BALAND} Z`;
  let uzunlik = 0;
  for (let i = 1; i < n; i++) uzunlik += Math.hypot(X(i) - X(i - 1), Y(nuqtalar[i].v) - Y(nuqtalar[i - 1].v));
  uzunlik = Math.ceil(uzunlik) + 8;

  const nishonlar = Array.from({ length: olcham.bolim + 1 }, (_, i) => olcham.cho - i * olcham.qadam);

  // Yorliqlar bir-birining ustiga chiqmasin: 44 px dan yaqin bo'lsa
  // tashlanadi, lekin oxirgi kun har doim qoladi (u eng muhimi).
  const xYorliqlar: number[] = [];
  for (const i of xIndekslar(nuqtalar, davr)) {
    const oxirgi = xYorliqlar[xYorliqlar.length - 1];
    if (oxirgi === undefined || X(i) - X(oxirgi) >= 44) xYorliqlar.push(i);
    else if (i === n - 1) xYorliqlar[xYorliqlar.length - 1] = i;
  }

  // Kunlik o'zgarish ustunlari.
  const engFarq = Math.max(1, ...farqlar.map((f) => Math.abs(f)));
  const qadamPx = n > 1 ? ichki / (n - 1) : ichki;
  const ustunEni = Math.max(2, Math.min(7, qadamPx * 0.42));

  const uid = `g${davr}-${birlik.replace(/[^a-z]/gi, "")}`;
  const tavsif = `${son(jami)} ${birlik}, ${davr} kunlik. O'rtacha ${son(ortacha)} kun. Eng ko'p ${son(nuqtalar[choqqiI].v)} — ${sanaToliq(nuqtalar[choqqiI].d)}.`;

  return (
    <div className="mt-[10px]">
      {/* Jami / o'rtacha qatori. O'zgarish nishoni panel sarlavhasida
          turadi (Figma 3.2b), shuning uchun bu yerda yo'q. */}
      <div className="mb-2 flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span className="text-[20px] font-bold leading-6 tabular-nums">{son(jami)}</span>
        <span className="text-[13px]" style={{ color: OCH_KUL }}>{birlik}</span>
        <span className="text-[13px]" style={{ color: HOSHIYA_QUYUQ }}>·</span>
        <span className="text-[13px]" style={{ color: OCH_KUL }}>o&apos;rtacha {son(ortacha)}/kun</span>
      </div>

      {/* Eng katta o'zgarish chiplari */}
      {(osishI > 0 || pasayishI > 0) && (
        <div className="mb-2 flex flex-wrap gap-2">
          {osishI > 0 && (
            <span className="flex h-6 items-center gap-1 rounded-full px-[10px] text-[12px] font-medium" style={{ background: "#eff8f4", color: YASHIL }}>
              <ArrowUp size={12} strokeWidth={2.8} aria-hidden />
              eng katta o&apos;sish <b className="font-semibold">{belgili(farqlar[osishI])}</b>
              <span style={{ color: HOSHIYA_QUYUQ }}>·</span>
              <b className="font-semibold">{sanaQisqa(nuqtalar[osishI].d, davr)}</b>
            </span>
          )}
          {pasayishI > 0 && (
            <span className="flex h-6 items-center gap-1 rounded-full px-[10px] text-[12px] font-medium" style={{ background: "#fcf2f2", color: QIZIL }}>
              <ArrowDown size={12} strokeWidth={2.8} aria-hidden />
              eng katta pasayish <b className="font-semibold">{belgili(farqlar[pasayishI])}</b>
              <span style={{ color: HOSHIYA_QUYUQ }}>·</span>
              <b className="font-semibold">{sanaQisqa(nuqtalar[pasayishI].d, davr)}</b>
            </span>
          )}
        </div>
      )}

      {/* Grafik bloki: qiymat o'qi + maydon + kunlik o'zgarish + sana o'qi */}
      <div className="relative" role="img" aria-label={tavsif}>
        <div className="flex" style={{ gap: CH_TIRQISH }}>
          {/* Qiymat o'qi */}
          <div className="relative shrink-0" style={{ width: CH_OQ, height: CH_BALAND }}>
            {nishonlar.map((v) => (
              <span
                key={v}
                className="absolute right-0 whitespace-nowrap text-[11px] leading-none tabular-nums"
                style={{ color: XIRA, top: Y(v), transform: "translateY(-50%)" }}
              >
                {v}
              </span>
            ))}
          </div>

          {/* Maydon */}
          <div ref={ref} className="relative min-w-0 flex-1" style={{ height: CH_BALAND }}>
            {W > 0 && (
              <svg width={W} height={CH_BALAND} className="block overflow-visible" aria-hidden>
                <defs>
                  <linearGradient id={`${uid}-soya`} x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={KO_K} stopOpacity="0.24" />
                    <stop offset="100%" stopColor={KO_K} stopOpacity="0.01" />
                  </linearGradient>
                </defs>

                {/* Uzuq to'r chiziqlari */}
                {nishonlar.map((v) => (
                  <line key={v} x1={0} y1={Y(v)} x2={W} y2={Y(v)} stroke={TUR} strokeWidth={1} strokeDasharray="3 4" />
                ))}

                {/* Soya — chiziq chizilib bo'lgach paydo bo'ladi */}
                <path
                  key={`${uid}-a-${n}`}
                  className="ib-anim ib-anim-area"
                  d={soya}
                  fill={`url(#${uid}-soya)`}
                  style={{ animationDelay: `${(kechikish + 0.80).toFixed(3)}s` }}
                />

                {/* Chiziq — chapdan o'ngga chiziladi */}
                <path
                  key={`${uid}-l-${n}`}
                  className="ib-anim ib-anim-line"
                  d={chiziq}
                  fill="none"
                  stroke={KO_K}
                  strokeWidth={2.5}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  style={{
                    strokeDasharray: uzunlik,
                    strokeDashoffset: 0,
                    ["--ib-len" as string]: String(uzunlik),
                    animationDelay: `${kechikish.toFixed(3)}s`,
                  } as React.CSSProperties}
                />

                {/* 7 kunda har bir kunga nuqta sig'adi */}
                {davr === 7 &&
                  nuqtalar.map((p, i) => (
                    <circle key={i} cx={X(i)} cy={Y(p.v)} r={4.5} fill="#ffffff" stroke={i === pastI && belgiBor ? XIRA : KO_K} strokeWidth={2.5} />
                  ))}

                {/* Eng past nuqta */}
                {belgiBor && davr !== 7 && (
                  <circle cx={X(pastI)} cy={Y(nuqtalar[pastI].v)} r={5} fill="#ffffff" stroke={XIRA} strokeWidth={2} />
                )}
                {/* Cho'qqi */}
                {belgiBor && <circle cx={X(choqqiI)} cy={Y(nuqtalar[choqqiI].v)} r={4.5} fill={KO_K} />}

                {/* Hover: uzuq vertikal chiziq va kattalashgan nuqta */}
                {ustida !== null && nuqtalar[ustida] && (
                  <>
                    <line x1={X(ustida)} y1={0} x2={X(ustida)} y2={CH_BALAND} stroke="#c9d8ef" strokeWidth={1.5} strokeDasharray="4 4" />
                    <circle cx={X(ustida)} cy={Y(nuqtalar[ustida].v)} r={5} fill={KO_K} stroke="#ffffff" strokeWidth={3} />
                  </>
                )}
              </svg>
            )}

            {/* Cho'qqi yorlig'i */}
            {W > 0 && belgiBor && (
              <div
                className="pointer-events-none absolute flex h-5 items-center rounded-md px-2 text-[12px] font-bold leading-none text-white"
                style={{
                  background: KO_K,
                  left: Math.max(0, Math.min(W - 44, X(choqqiI) - 22)),
                  top: Math.max(0, Math.min(CH_BALAND - 20, Y(nuqtalar[choqqiI].v) - 26)),
                }}
              >
                {son(nuqtalar[choqqiI].v)}
              </div>
            )}

            {/* Sichqoncha maydoni */}
            {W > 0 && (
              <div
                className="absolute inset-0"
                onPointerMove={(e) => {
                  const r = e.currentTarget.getBoundingClientRect();
                  const x = e.clientX - r.left;
                  const i = n <= 1 ? 0 : Math.round(((x - CH_CHET) / Math.max(1, ichki)) * (n - 1));
                  setUstida(Math.max(0, Math.min(n - 1, i)));
                }}
                onPointerLeave={() => setUstida(null)}
              />
            )}

            {/* Hover oynasi */}
            {W >= 220 && ustida !== null && nuqtalar[ustida] && (
              <div
                className="pointer-events-none absolute z-10 w-[193px] rounded-[10px] px-[13px] py-3"
                style={{
                  background: IK,
                  top: 6,
                  // O'ng yarmida bo'lsa oyna chapga o'tadi, aks holda maydondan chiqib ketardi.
                  left: X(ustida) > W / 2 ? Math.max(0, X(ustida) - 12 - 193) : Math.min(W - 193, X(ustida) + 12),
                }}
              >
                <div className="text-[11px] leading-none" style={{ color: "#97a1b4" }}>{sanaToliq(nuqtalar[ustida].d)}</div>
                <div className="mt-2 flex items-baseline gap-2">
                  <span className="text-[20px] font-bold leading-none text-white tabular-nums">{son(nuqtalar[ustida].v)}</span>
                  <span className="text-[11px] leading-none" style={{ color: "#97a1b4" }}>{birlik}</span>
                </div>
                <div className="my-[10px] h-px" style={{ background: "rgba(255,255,255,0.12)" }} />
                <div
                  className="flex items-center gap-1 text-[11px] leading-none"
                  style={{ color: ustida === 0 ? "#97a1b4" : farqlar[ustida] < 0 ? "#ff8a8f" : "#6ee7b7" }}
                >
                  {ustida === 0 ? (
                    "davr boshlanishi"
                  ) : (
                    <>
                      {farqlar[ustida] < 0 ? <ArrowDown size={12} strokeWidth={2.6} aria-hidden /> : <ArrowUp size={12} strokeWidth={2.6} aria-hidden />}
                      {belgili(farqlar[ustida])}
                      {nuqtalar[ustida - 1].v > 0 && ` (${foiz((farqlar[ustida] / nuqtalar[ustida - 1].v) * 100)})`} kechagiga nisbatan
                    </>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Kunlik o'zgarish */}
        <div className="mt-3 text-[9px] leading-3" style={{ color: XIRA, paddingLeft: CH_OQ + CH_TIRQISH }}>
          kunlik o&apos;zgarish
        </div>
        <div className="mt-0.5" style={{ height: CH_DELTA, paddingLeft: CH_OQ + CH_TIRQISH }}>
          {W > 0 && (
            <svg width={W} height={CH_DELTA} className="block" aria-hidden>
              <line x1={0} y1={CH_DELTA / 2} x2={W} y2={CH_DELTA / 2} stroke="#edeff6" strokeWidth={1} />
              {farqlar.map((f, i) => {
                if (i === 0 || f === 0) return null;
                const h = Math.max(3, (Math.abs(f) / engFarq) * 10);
                const musbat = f > 0;
                return (
                  <rect
                    key={i}
                    x={X(i) - ustunEni / 2}
                    y={musbat ? CH_DELTA / 2 - h : CH_DELTA / 2}
                    width={ustunEni}
                    height={h}
                    rx={Math.min(1.5, ustunEni / 2)}
                    fill={musbat ? YASHIL : QIZIL}
                    opacity={ustida === null || ustida === i ? 1 : 0.3}
                  />
                );
              })}
            </svg>
          )}
        </div>

        {/* Sana o'qi */}
        <div className="relative mt-2 h-[14px]" style={{ marginLeft: CH_OQ + CH_TIRQISH }}>
          {W > 0 &&
            xYorliqlar.map((i) => (
              <span
                key={i}
                className="absolute whitespace-nowrap text-[11px] leading-[14px]"
                style={{
                  left: Math.max(16, Math.min(W - 16, X(i))),
                  transform: "translateX(-50%)",
                  color: i === n - 1 ? IK : OCH_KUL,
                  fontWeight: i === n - 1 ? 600 : 500,
                }}
              >
                {sanaQisqa(nuqtalar[i].d, davr)}
              </span>
            ))}
          {/* Hover qilingan kun alohida to'q yorliq bilan belgilanadi */}
          {W > 0 && ustida !== null && nuqtalar[ustida] && (
            <span
              className="absolute flex h-[17px] items-center rounded-md px-[6px] text-[11px] font-semibold leading-none text-white"
              style={{
                background: IK,
                left: Math.max(21, Math.min(W - 21, X(ustida))),
                transform: "translateX(-50%)",
                top: -1,
              }}
            >
              {sanaQisqa(nuqtalar[ustida].d, davr)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

// Sana o'qida qaysi kunlar yorliq oladi (Figma 3.2b · A/B/C panellari):
// 7 kun — har biri; 30 kun — har 6-kun va oxirgisi; 90 kun — birinchi kun,
// har oyning boshi va oxirgi kun.
function xIndekslar(nuqtalar: Nuqta[], davr: Davr): number[] {
  const n = nuqtalar.length;
  if (n === 0) return [];
  if (davr === 7) return nuqtalar.map((_, i) => i);
  const s = new Set<number>([0, n - 1]);
  if (davr === 30) {
    for (let i = 0; i < n; i += 6) s.add(i);
  } else {
    nuqtalar.forEach((p, i) => { if (yaroqli(p.d) && p.d.getDate() === 1) s.add(i); });
  }
  return Array.from(s).sort((a, b) => a - b);
}
