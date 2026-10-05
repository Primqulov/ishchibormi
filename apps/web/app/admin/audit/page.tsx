"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CalendarDays,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  Eye,
  Filter,
  RotateCw,
  ScrollText,
  Search,
  ShieldOff,
  X,
} from "lucide-react";
import { APIError, AdminAudit, Paged, api } from "@/lib/api";
import {
  AVATAR_FON,
  HOSHIYA,
  HOSHIYA_OCH,
  HOSHIYA_QUYUQ,
  IK,
  KO_K,
  KUL,
  OCH_KUL,
  QIZIL,
  QIZIL_FON,
  QIZIL_HOSHIYA,
  SARLAVHA_FON,
  SOYA,
  XIRA,
  XIRA_QUYUQ,
  tugma,
} from "@/components/admin/ui";

/* ─────────────────────────────────────────────────────────────────────
   Figma: "3.11 · Audit log — ro'yxat (1440×1024)" va
          "3.11a · Audit log — 24 ta amal turi va holatlar".

   O'lchamlar Figma'dan: sarlavha kartasi 83, filtr maydonlari h40
   (ustida 12px yorliq), jadval sarlavhasi 46, qator 61+1px chegara,
   sahifa raqamlari 34×34, sahifada 30 tadan yozuv.

   # BU EKRAN FAQAT O'QISH UCHUN

   Jurnalda tahrirlash, o'chirish yoki eksport tugmasi YO'Q — na bu
   yerda, na backendda (`GET /admin/audit` yagona yo'l). Sabab oddiy:
   audit log — o'zini himoya qilolmaydigan yagona jadval; uni tozalay
   oladigan odam o'z izini ham tozalay oladi.

   Marshrut `RequireRole("moderator")` ichida: superadmin va moderator
   ko'radi, support ko'rmaydi. 403 kelsa buni aniq aytamiz, "server
   javob bermadi" deb yashirmaymiz.

   # XAVFSIZLIK QARORLARI

   · Filtrga faqat KATALOGDAGI amal kodi yuboriladi (quyidagi `AMAL`).
     Foydalanuvchi kiritgan matn hech qachon so'rovga tushmaydi; server
     ham o'z tomonidan `^[a-z0-9_]{1,64}$` bilan tekshiradi.
   · Katalog backend YOZADIGAN 30 ta kodning hammasini qamraydi — 24
     emas. Dizaynda 6 tasi yo'q edi: login_throttled, 2fa_throttled,
     session_idle_expired, user_unblock, moderation_ban_lift,
     category_icon_upload. Ulardan ikkitasi to'g'ridan-to'g'ri hujum
     signali (parol yoki 2FA kodini terib ko'rish cheklandi) — ularni
     filtrdan tushirib qoldirish xavfsizlik regressiyasi bo'lardi.
     Guruhlar soni o'zgarmadi: yangi kodlar o'z guruhiga qo'shildi,
     ya'ni "bitta guruh — bitta rang" qoidasi buzilmadi.
   · "Barchasi (N xil amal)" dagi N katalogdan HISOBLANADI, qo'lda
     yozilmaydi — shunda yorliq hech qachon yolg'on gapirmaydi.
   · Har so'rovga tartib raqami beriladi: sekin qaytgan eski javob
     yangi filtr natijasini bosib ketmaydi (audit ekranida eskirgan
     natija — noto'g'ri javob).
   · Nishon nomlarini (turkum nomi, @login) SERVER qaytaradi. Ilgari
     sahifa buning uchun butun admin ro'yxatini tortardi — u faqat
     superadminga ochiq, ya'ni moderatorda ishlamasdi, ustiga-ustak
     kosmetik yorliq uchun brauzerga butun xodimlar ro'yxati borardi.
   ───────────────────────────────────────────────────────────────── */

const LIMIT = 30; // Figma: bu jadval boshqalardan farqli — sahifada 30 ta.
const ZEBRA = "#f8f9ff"; // juft qatorlar foni
const AVATAR_KO_K = "#dce9ff";

const FOKUS =
  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[#004ac6]";

/* ── Amal katalogi ─────────────────────────────────────────────────── */

type GuruhKod =
  | "sessiya"
  | "tarqatma"
  | "turkum"
  | "admin"
  | "xavfsizlik"
  | "foydalanuvchi"
  | "elon"
  | "xatolik"
  | "eksport";

/** Har guruh o'z rangiga ega; rang jadvalda ham, chipda ham bir xil. */
const GURUH: Record<GuruhKod, { nomi: string; rang: string; fon: string }> = {
  sessiya: { nomi: "Sessiya", rang: "#737686", fon: "#eef1fb" },
  tarqatma: { nomi: "Tarqatma", rang: "#004ac6", fon: "#dce9ff" },
  turkum: { nomi: "Turkumlar", rang: "#e8890c", fon: "#fcf3e4" },
  admin: { nomi: "Adminlar", rang: "#7a5af8", fon: "#eeeafe" },
  xavfsizlik: { nomi: "Xavfsizlik", rang: "#1fa463", fon: "#e6f5ed" },
  foydalanuvchi: { nomi: "Foydalanuvchilar", rang: "#e5484d", fon: "#fcebec" },
  elon: { nomi: "E'lonlar", rang: "#0e9aa7", fon: "#e2f5f6" },
  // Xatoliklar jurnali (3.12) — o'z guruhi: bu tizimning o'z holati ustidagi
  // amal, xodim yoki mazmun ustidagi emas. Rang ataylab sovuq-quyuq
  // ("diagnostika"), sessiya kulidan bir necha pog'ona quyuqroq.
  xatolik: { nomi: "Xatoliklar", rang: "#334155", fon: "#eceff5" },
  eksport: { nomi: "Eksport", rang: "#b0338c", fon: "#fbeaf5" },
};
const GURUH_TARTIBI: GuruhKod[] = [
  "sessiya",
  "tarqatma",
  "turkum",
  "admin",
  "xavfsizlik",
  "foydalanuvchi",
  "elon",
  "xatolik",
  "eksport",
];

/**
 * Maqsad ustunining turi. `xom: true` — nishon matn (login, sarlavha, fayl
 * kaliti), o'zgartirmasdan ko'rsatiladi. `xom: false` — nishon ObjectID:
 * server nom topib bergan bo'lsa nom, aks holda `…a1b2c3`.
 */
type MaqsadTur =
  | "yoq"
  | "login"
  | "sarlavha"
  | "fayl"
  | "qamrov"
  | "turkum"
  | "admin"
  | "foydalanuvchi"
  | "elon"
  | "tarqatma"
  | "xatolik";

const MAQSAD: Record<MaqsadTur, { yorliq: string; xom: boolean }> = {
  yoq: { yorliq: "", xom: false },
  login: { yorliq: "Login", xom: true },
  sarlavha: { yorliq: "Sarlavha", xom: true },
  fayl: { yorliq: "Fayl", xom: true },
  qamrov: { yorliq: "Qamrov", xom: true },
  turkum: { yorliq: "Turkum", xom: false },
  admin: { yorliq: "Admin", xom: false },
  foydalanuvchi: { yorliq: "Foydalanuvchi", xom: false },
  elon: { yorliq: "E'lon", xom: false },
  tarqatma: { yorliq: "Tarqatma", xom: false },
  // Server nishon id'sini `ERR-2F91C4` yorlig'iga aylantiradi (audit.go ·
  // targetNames) — auditdagi qatorni "Xatoliklar" ekranidagi qator bilan
  // ko'z bilan solishtirish uchun.
  xatolik: { yorliq: "Xatolik", xom: false },
};

type AmalMeta = { yorliq: string; guruh: GuruhKod; maqsad: MaqsadTur };

/**
 * Backend yozadigan HAMMA amal kodi. Ro'yxat `h.audit` / `h.auditRaw`
 * chaqiruvlaridan olingan; `internal/admin/audit_filter_test.go` uni manba
 * kodi bilan solishtirib turadi — yangi amal qo'shilib bu yerga yozilmasa
 * test yiqiladi.
 *
 * Maqsad turi nishonning HAQIQIY mazmuniga qarab tanlangan:
 * · login_success / logout / session_idle_expired — nishon o'sha adminning
 *   o'z logini, u "Admin" ustunida allaqachon turibdi → takrorlamaymiz.
 * · login_failed / login_throttled — aksincha, KO'RSATAMIZ: bunda amalni
 *   kim qilgani noma'lum bo'lishi mumkin (mavjud bo'lmagan hisob, tugagan
 *   urinish byudjeti), va terib ko'rilgan login — yagona iz.
 */
const AMAL: Record<string, AmalMeta> = {
  // ── Sessiya ──
  login_success: { yorliq: "Tizimga kirdi", guruh: "sessiya", maqsad: "yoq" },
  login_failed: {
    yorliq: "Kirish urinishi (muvaffaqiyatsiz)",
    guruh: "sessiya",
    maqsad: "login",
  },
  login_throttled: {
    yorliq: "Kirish urinishlari cheklandi",
    guruh: "sessiya",
    maqsad: "login",
  },
  logout: { yorliq: "Tizimdan chiqdi", guruh: "sessiya", maqsad: "yoq" },
  // Panelning bosh sahifasi ochilgani. Server soatiga bir marta yozadi
  // (internal/admin/stats.go · dashboardAuditWindow) — dashboard o'zi
  // qayta so'ralgani uchun har so'rovga yozuv qo'yilsa jurnal shu bitta
  // amaldan iborat bo'lib qolardi. Izohda klient platformasi turadi.
  dashboard_viewed: {
    yorliq: "Bosh sahifani ochdi",
    guruh: "sessiya",
    maqsad: "yoq",
  },
  session_idle_expired: {
    yorliq: "Sessiya harakatsizlikdan yopildi",
    guruh: "sessiya",
    maqsad: "yoq",
  },
  // ── Tarqatma ──
  broadcast: { yorliq: "Tarqatma yubordi", guruh: "tarqatma", maqsad: "sarlavha" },
  broadcast_schedule: {
    yorliq: "Tarqatma rejalashtirdi",
    guruh: "tarqatma",
    maqsad: "sarlavha",
  },
  broadcast_cancel: {
    yorliq: "Tarqatmani bekor qildi",
    guruh: "tarqatma",
    maqsad: "tarqatma",
  },
  // ── Turkumlar ──
  category_create: { yorliq: "Turkum qo'shdi", guruh: "turkum", maqsad: "turkum" },
  category_update: { yorliq: "Turkumni tahrirladi", guruh: "turkum", maqsad: "turkum" },
  category_delete: { yorliq: "Turkumni o'chirdi", guruh: "turkum", maqsad: "turkum" },
  category_active: {
    yorliq: "Turkum holatini o'zgartirdi",
    guruh: "turkum",
    maqsad: "turkum",
  },
  category_icon_upload: {
    yorliq: "Turkum ikonkasini yukladi",
    guruh: "turkum",
    maqsad: "fayl",
  },
  // ── Adminlar ──
  admin_create: { yorliq: "Yangi admin qo'shdi", guruh: "admin", maqsad: "admin" },
  admin_update: { yorliq: "Adminni tahrirladi", guruh: "admin", maqsad: "admin" },
  admin_delete: { yorliq: "Adminni o'chirdi", guruh: "admin", maqsad: "admin" },
  // ── Xavfsizlik ──
  "2fa_enable": { yorliq: "2FA'ni yoqdi", guruh: "xavfsizlik", maqsad: "admin" },
  "2fa_disable": { yorliq: "2FA'ni o'chirdi", guruh: "xavfsizlik", maqsad: "admin" },
  "2fa_throttled": {
    yorliq: "2FA kod urinishlari cheklandi",
    guruh: "xavfsizlik",
    maqsad: "admin",
  },
  // ── Foydalanuvchilar ──
  user_block: {
    yorliq: "Foydalanuvchini blokladi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  user_unblock: {
    yorliq: "Foydalanuvchi blokini yechdi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  user_delete: {
    yorliq: "Foydalanuvchini o'chirdi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  user_notify: {
    yorliq: "Foydalanuvchiga xabar yubordi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  user_verify: {
    yorliq: "Foydalanuvchini tasdiqladi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  "avatar.download": {
    yorliq: "Profil rasmini yuklab oldi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  "avatar.delete": {
    yorliq: "Profil rasmini o'chirdi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  moderation_ban: {
    yorliq: "Foydalanuvchi avtomatik bloklandi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  moderation_ban_lift: {
    yorliq: "Avtomatik blokni bekor qildi",
    guruh: "foydalanuvchi",
    maqsad: "foydalanuvchi",
  },
  // ── E'lonlar ──
  elon_delete: { yorliq: "E'lonni o'chirdi", guruh: "elon", maqsad: "elon" },
  elon_status: { yorliq: "E'lon holatini o'zgartirdi", guruh: "elon", maqsad: "elon" },
  // «Muddati o'tgan ishlar» kaliti (E'lonlar sahifasi). Nishon bitta —
  // butun feed, shuning uchun maqsad ustuni bo'sh; yo'nalish izohda turadi.
  feed_show_expired: {
    yorliq: "Muddati o'tgan ishlar ko'rinishini o'zgartirdi",
    guruh: "elon",
    maqsad: "yoq",
  },
  // ── Xatoliklar (3.12) ──
  // Ikkita alohida kod bir sababga ko'ra: `error_ignore` — nosozlikni
  // ro'yxatdan va ogohlantirishlardan OLIB TASHLASH amali. Uni oddiy holat
  // o'zgarishidan ajratib filtrlash mumkin bo'lishi kerak.
  error_status: {
    yorliq: "Xatolik holatini o'zgartirdi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  error_ignore: {
    yorliq: "Xatolikni e'tiborsiz qoldirdi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  error_assign: {
    yorliq: "Xatolikka mas'ul belgiladi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  error_note: {
    yorliq: "Xatolikka izoh qo'shdi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  // `error_export` — batafsil ekrandagi "AI uchun kontekst" nusxalash yoki
  // yuklab olish. Matn niqoblangan bo'lsa ham, u diagnostikaning eng zich
  // to'plami: kim va qachon olganini bilish kerak.
  error_export: {
    yorliq: "Xatolik kontekstini eksport qildi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  error_telegram: {
    yorliq: "Xatolikni Telegram'ga yubordi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  // `error_ai` — "Sababini aniqla". Eksport bilan AYNAN bir xil niqoblangan
  // matn tashqi xizmatga (Gemini) chiqadi, shu sababli u ham auditda
  // ko'rinadi: izohda model nomi va sarflangan token soni turadi.
  error_ai: {
    yorliq: "Xatolik sababini AI orqali tahlil qildi",
    guruh: "xatolik",
    maqsad: "xatolik",
  },
  // ── Eksport ──
  export_users: {
    yorliq: "Foydalanuvchilarni CSV eksport qildi",
    guruh: "eksport",
    maqsad: "yoq",
  },
  export_elons: {
    yorliq: "E'lonlarni CSV eksport qildi",
    guruh: "eksport",
    maqsad: "yoq",
  },
  export_applications: {
    yorliq: "Arizalarni CSV eksport qildi",
    guruh: "eksport",
    maqsad: "qamrov",
  },
};

const AMAL_SONI = Object.keys(AMAL).length;

/** Katalogni guruhlarga bo'lib beradi — <optgroup> shu tartibda chiziladi. */
const GURUHLANGAN = GURUH_TARTIBI.map((g) => ({
  guruh: g,
  amallar: Object.entries(AMAL)
    .filter(([, m]) => m.guruh === g)
    .map(([kod, m]) => ({ kod, yorliq: m.yorliq })),
})).filter((x) => x.amallar.length > 0);

/* ── Kichik yordamchilar ───────────────────────────────────────────── */

const p2 = (n: number) => String(n).padStart(2, "0");

function son(v: number): string {
  if (!Number.isFinite(v)) return "—";
  return Math.max(0, Math.round(v))
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

function sana(d: Date): string {
  return `${p2(d.getDate())}.${p2(d.getMonth() + 1)}.${d.getFullYear()}`;
}
function soat(d: Date): string {
  return `${p2(d.getHours())}:${p2(d.getMinutes())}`;
}
/** "2026-08-01" (input[type=date]) → "01.08.2026". */
function sanaMatn(iso: string): string {
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

function boshHarflar(nom: string): string {
  const q = nom.trim().split(/\s+/).filter(Boolean);
  if (q.length === 0) return "?";
  if (q.length === 1) return q[0].slice(0, 2).toLocaleUpperCase("uz");
  return (q[0][0] + q[1][0]).toLocaleUpperCase("uz");
}

/** Sahifa raqamlari: 1 · joriy atrofidagi 3 ta · oxirgi, orasida "…". */
function sahifaRoyxati(joriy: number, jami: number): (number | "…")[] {
  if (jami <= 7) return Array.from({ length: jami }, (_, i) => i + 1);
  const s = new Set<number>([1, jami, joriy - 1, joriy, joriy + 1]);
  if (joriy <= 3) {
    s.add(2);
    s.add(3);
  }
  if (joriy >= jami - 2) {
    s.add(jami - 1);
    s.add(jami - 2);
  }
  const raqamlar = [...s].filter((n) => n >= 1 && n <= jami).sort((a, b) => a - b);
  const chiqish: (number | "…")[] = [];
  raqamlar.forEach((n, i) => {
    if (i > 0 && n - raqamlar[i - 1] > 1) chiqish.push("…");
    chiqish.push(n);
  });
  return chiqish;
}

/* ── Sahifa ────────────────────────────────────────────────────────── */

export default function AuditPage() {
  const [data, setData] = useState<Paged<AdminAudit> | null>(null);
  const [yuklanmoqda, setYuklanmoqda] = useState(true);
  const [xato, setXato] = useState<APIError | null>(null);

  const [amal, setAmal] = useState("");
  const [dan, setDan] = useState("");
  const [gacha, setGacha] = useState("");
  const [page, setPage] = useState(1);

  // Faqat katalogdagi kod so'rovga tushadi.
  const amalQiymati = amal && AMAL[amal] ? amal : "";
  // "Dan" "Gacha"dan katta bo'lsa — so'rov YUBORILMAYDI. ISO satrlarni
  // leksikografik solishtirish sana solishtirishga teng.
  const sanaXato = Boolean(dan && gacha && dan > gacha);
  const filtrBor = Boolean(amalQiymati || dan || gacha);

  const soravRaqami = useRef(0);

  const load = useCallback(async () => {
    if (sanaXato) return;
    const men = ++soravRaqami.current;
    setYuklanmoqda(true);
    try {
      const params = new URLSearchParams({
        page: String(Math.max(1, Math.floor(page))),
        limit: String(LIMIT),
      });
      if (amalQiymati) params.set("action", amalQiymati);
      if (dan) {
        const d = new Date(`${dan}T00:00:00`);
        if (!Number.isNaN(d.getTime())) params.set("from", d.toISOString());
      }
      if (gacha) {
        // Kun OXIRIGACHA: aks holda "28.08" tanlagan admin o'sha kunning
        // yozuvlarini umuman ko'rmasdi.
        const d = new Date(`${gacha}T23:59:59.999`);
        if (!Number.isNaN(d.getTime())) params.set("to", d.toISOString());
      }
      const res = await api.get<Paged<AdminAudit>>(
        `/api/admin/audit?${params.toString()}`,
        { auth: "admin" } as any,
      );
      if (men !== soravRaqami.current) return;
      setData(res);
      setXato(null);
    } catch (e) {
      if (men !== soravRaqami.current) return;
      setData(null);
      setXato((e as APIError) ?? null);
    } finally {
      if (men === soravRaqami.current) setYuklanmoqda(false);
    }
  }, [page, amalQiymati, dan, gacha, sanaXato]);

  useEffect(() => {
    load();
  }, [load]);

  // Filtr o'zgarsa — birinchi sahifaga: 7-sahifada turib filtr almashtirish
  // aks holda bo'sh natija berardi.
  useEffect(() => {
    setPage(1);
  }, [amalQiymati, dan, gacha]);

  const total = data?.total ?? 0;
  const sahifalar = Math.max(1, Math.ceil(total / LIMIT));
  const qatorlar = data?.items ?? [];
  const boshi = total === 0 ? 0 : (page - 1) * LIMIT + 1;
  const oxiri = Math.min(page * LIMIT, total);

  const tozala = useCallback(() => {
    setAmal("");
    setDan("");
    setGacha("");
    setPage(1);
  }, []);

  const karta: React.CSSProperties = useMemo(
    () => ({ boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${SOYA}` }),
    [],
  );
  const maydonHoshiya = (xatoli?: boolean): React.CSSProperties => ({
    boxShadow: `inset 0 0 0 ${xatoli ? 1.5 : 1}px ${xatoli ? QIZIL_HOSHIYA : HOSHIYA_QUYUQ}`,
    color: IK,
  });

  const ruxsatYoq = xato?.code === "forbidden";
  const tozalashUslub = tugma("ikkilamchi", { ochiq: !filtrBor });

  return (
    <div className="flex flex-col gap-4">
      {/* ── Sarlavha kartasi ───────────────────────────────────────── */}
      <div
        className="flex h-[83px] items-center gap-[14px] rounded-2xl bg-white px-5"
        style={karta}
      >
        <span
          aria-hidden
          className="grid h-9 w-9 shrink-0 place-items-center rounded-[10px]"
          style={{ background: AVATAR_KO_K }}
        >
          <ScrollText size={18} color={KO_K} />
        </span>
        <div className="min-w-0">
          <h1 className="text-[18px] font-bold leading-6" style={{ color: IK }}>
            Audit log
          </h1>
          <p className="mt-[2px] text-[12px] leading-4" style={{ color: OCH_KUL }}>
            Adminlar amallari tarixi
            {data ? ` · ${filtrBor ? "filtrga mos" : "jami"} ${son(total)} ta yozuv` : ""}
          </p>
        </div>
        {/* Bu ekranda o'zgartirish tugmalari yo'q — buni yashirmay aytamiz. */}
        <span
          className="ml-auto inline-flex h-[25px] shrink-0 items-center gap-[6px] rounded-full px-[10px]"
          style={{ background: AVATAR_FON }}
          title="Jurnal yozuvlari o'zgartirilmaydi va o'chirilmaydi"
        >
          <Eye size={13} color={OCH_KUL} aria-hidden />
          <span className="whitespace-nowrap text-[12px] leading-4" style={{ color: OCH_KUL }}>
            Faqat ko'rish uchun
          </span>
        </span>
      </div>

      {/* ── Filtr + jadval kartasi ─────────────────────────────────── */}
      <div className="overflow-hidden rounded-[14px] bg-white" style={karta}>
        {/* Filtr yo'lagi. Yuklanayotganda ham bosilaveradi — bloklamaymiz. */}
        <div className="flex flex-wrap items-end gap-3 px-5 pb-[15px] pt-[15px]">
          {/* Amal */}
          <div className="w-[250px] min-w-[190px] flex-shrink-0">
            <label
              htmlFor="audit-amal"
              className="mb-[6px] block text-[12px] leading-4"
              style={{ color: OCH_KUL }}
            >
              Amal
            </label>
            <div className="relative">
              <Filter
                size={15}
                color={amalQiymati ? GURUH[AMAL[amalQiymati].guruh].rang : OCH_KUL}
                aria-hidden
                className="pointer-events-none absolute left-[11px] top-1/2 -translate-y-1/2"
              />
              <select
                id="audit-amal"
                value={amal}
                onChange={(e) => setAmal(e.target.value)}
                className={`h-10 w-full appearance-none rounded-[10px] bg-white pl-[34px] pr-[30px] text-[13px] leading-[18px] outline-none ${FOKUS}`}
                style={maydonHoshiya()}
              >
                <option value="">{`Barchasi (${AMAL_SONI} xil amal)`}</option>
                {GURUHLANGAN.map((g) => (
                  <optgroup key={g.guruh} label={GURUH[g.guruh].nomi.toUpperCase()}>
                    {g.amallar.map((a) => (
                      <option key={a.kod} value={a.kod}>
                        {a.yorliq}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <ChevronDown
                size={15}
                color={OCH_KUL}
                aria-hidden
                className="pointer-events-none absolute right-[10px] top-1/2 -translate-y-1/2"
              />
            </div>
          </div>

          <SanaMaydon
            id="audit-dan"
            nomi="Dan"
            qiymat={dan}
            ozgardi={setDan}
            xatoli={sanaXato}
            uslub={maydonHoshiya(sanaXato)}
          />
          <SanaMaydon
            id="audit-gacha"
            nomi="Gacha"
            qiymat={gacha}
            ozgardi={setGacha}
            xatoli={sanaXato}
            uslub={maydonHoshiya(sanaXato)}
          />

          <button
            type="button"
            onClick={tozala}
            disabled={!filtrBor}
            className={`${tozalashUslub.className} w-[105px]`}
            style={{ ...tozalashUslub.style, height: 40 }}
          >
            <X size={15} aria-hidden />
            Tozalash
          </button>

          <div
            className="ml-auto self-end pb-[11px] text-[13px] font-semibold leading-[18px]"
            style={{ color: IK }}
          >
            Jami: {data ? son(total) : "—"}
          </div>
        </div>

        {/* Sana oralig'i xatosi — so'rov umuman yuborilmaydi. */}
        {sanaXato && (
          <div className="px-5 pb-[15px]">
            <div
              role="alert"
              className="inline-flex items-center gap-[8px] rounded-[9px] px-3 py-[9px]"
              style={{
                background: QIZIL_FON,
                boxShadow: `inset 0 0 0 1px ${QIZIL_HOSHIYA}`,
              }}
            >
              <CircleAlert size={15} color={QIZIL} aria-hidden />
              <span className="text-[13px] leading-[18px]" style={{ color: QIZIL }}>
                Sana oralig&apos;i noto&apos;g&apos;ri — «Dan» «Gacha»dan keyin turibdi.
              </span>
            </div>
          </div>
        )}

        {/* Faol filtr chiplari */}
        {filtrBor && !sanaXato && (
          <div className="flex flex-wrap items-center gap-2 px-5 pb-[15px]">
            {amalQiymati && (
              <Chip
                matn={AMAL[amalQiymati].yorliq}
                rang={GURUH[AMAL[amalQiymati].guruh].rang}
                fon={GURUH[AMAL[amalQiymati].guruh].fon}
                olib={() => setAmal("")}
              />
            )}
            {(dan || gacha) && (
              <Chip
                matn={
                  dan && gacha
                    ? `${sanaMatn(dan)} — ${sanaMatn(gacha)}`
                    : dan
                      ? `${sanaMatn(dan)} dan`
                      : `${sanaMatn(gacha)} gacha`
                }
                rang={KO_K}
                fon={AVATAR_FON}
                olib={() => {
                  setDan("");
                  setGacha("");
                }}
              />
            )}
          </div>
        )}

        <div className="h-px" style={{ background: HOSHIYA }} />

        {/* ── Jadval ───────────────────────────────────────────────── */}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1000px] table-fixed border-collapse text-[14px]">
            <colgroup>
              <col style={{ width: `${(187 / 1127) * 100}%` }} />
              <col style={{ width: `${(184 / 1127) * 100}%` }} />
              <col style={{ width: `${(276 / 1127) * 100}%` }} />
              <col style={{ width: `${(254 / 1127) * 100}%` }} />
              <col style={{ width: `${(226 / 1127) * 100}%` }} />
            </colgroup>
            <thead>
              <tr style={{ background: SARLAVHA_FON }}>
                {["Vaqt", "Admin", "Amal", "Maqsad", "Tafsilot"].map((h, i) => (
                  <th
                    key={h}
                    className={`h-[46px] text-left text-[12px] font-semibold leading-4 ${
                      i === 0 ? "pl-5 pr-3" : "px-3"
                    }`}
                    style={{ color: KUL }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {yuklanmoqda ? (
                // Skelet: sahifadagi qator soniga teng, ya'ni natija kelganda
                // jadval balandligi sakramaydi. Filtr paneli bloklanmaydi —
                // yuklanish paytida ham tanlash va tozalash ishlayveradi.
                Array.from({ length: LIMIT }).map((_, i) => (
                  <tr
                    key={`skelet-${i}`}
                    className="border-b last:border-b-0"
                    style={{ borderColor: HOSHIYA, background: i % 2 ? ZEBRA : "#fff" }}
                  >
                    {[70, 120, 160, 110, 180].map((w, j) => (
                      <td key={j} className={`h-[61px] ${j === 0 ? "pl-5 pr-3" : "px-3"}`}>
                        <span
                          className="block h-[10px] animate-pulse rounded-full"
                          style={{ width: w, background: HOSHIYA_OCH }}
                        />
                      </td>
                    ))}
                  </tr>
                ))
              ) : xato ? (
                <tr>
                  <td colSpan={5} className="px-5 py-[52px]">
                    <Holat
                      ikon={
                        ruxsatYoq ? (
                          <ShieldOff size={34} color={XIRA} aria-hidden />
                        ) : (
                          <CircleAlert size={34} color={QIZIL} aria-hidden />
                        )
                      }
                      sarlavha={ruxsatYoq ? "Ruxsat yo'q" : "Server javob bermadi"}
                      sarlavhaRang={ruxsatYoq ? IK : QIZIL}
                      tavsif={
                        ruxsatYoq
                          ? "Audit log faqat superadmin va moderator uchun ochiq."
                          : xato.message ||
                            "Tarixni yuklab bo'lmadi. Qaytadan urinib ko'ring."
                      }
                      amal={
                        ruxsatYoq ? null : (
                          <button
                            type="button"
                            onClick={load}
                            className={tugma("asosiy").className}
                            style={{ ...tugma("asosiy").style, height: 38 }}
                          >
                            <RotateCw size={15} aria-hidden />
                            Qayta urinish
                          </button>
                        )
                      }
                    />
                  </td>
                </tr>
              ) : qatorlar.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-5 py-[52px]">
                    <Holat
                      ikon={<Search size={34} color={XIRA} aria-hidden />}
                      sarlavha="Yozuv topilmadi"
                      sarlavhaRang={IK}
                      tavsif={
                        filtrBor
                          ? "Tanlangan amal turi yoki sana oralig'ida yozuv yo'q."
                          : "Hali birorta amal jurnalga yozilmagan."
                      }
                      amal={
                        filtrBor ? (
                          <button
                            type="button"
                            onClick={tozala}
                            className={tugma("ikkilamchi").className}
                            style={{ ...tugma("ikkilamchi").style, height: 38 }}
                          >
                            <X size={15} aria-hidden />
                            Filtrlarni tozalash
                          </button>
                        ) : null
                      }
                    />
                  </td>
                </tr>
              ) : (
                qatorlar.map((a, i) => <Qator key={a.id} a={a} juft={i % 2 === 1} />)
              )}
            </tbody>
          </table>
        </div>

        {/* ── Pastki yo'lak ────────────────────────────────────────── */}
        {/* Qayta yuklashda ham joyida qoladi (data bor) — jadval skeletga
            o'tganda pastki yo'lak sakrab yo'qolmasin. */}
        {(qatorlar.length > 0 || (yuklanmoqda && data !== null)) && (
          <>
            <div className="h-px" style={{ background: HOSHIYA }} />
            <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-[10px]">
              <span className="text-[13px] leading-[18px]" style={{ color: OCH_KUL }}>
                {boshi}–{oxiri} / {son(total)} ta yozuv · sahifada {LIMIT} tadan
              </span>
              {sahifalar > 1 && (
                <nav className="flex items-center gap-[11px]" aria-label="Sahifalar">
                  <SahifaTugma
                    belgi={<ChevronLeft size={16} aria-hidden />}
                    nomi="Oldingi sahifa"
                    ochiq={page <= 1}
                    bosildi={() => setPage((v) => Math.max(1, v - 1))}
                  />
                  {sahifaRoyxati(page, sahifalar).map((n, i) =>
                    n === "…" ? (
                      <span
                        key={`uch-nuqta-${i}`}
                        aria-hidden
                        className="grid h-[34px] w-[34px] place-items-center text-[13px]"
                        style={{ color: XIRA_QUYUQ }}
                      >
                        …
                      </span>
                    ) : (
                      <SahifaTugma
                        key={n}
                        belgi={n}
                        nomi={`${n}-sahifa`}
                        faol={n === page}
                        bosildi={() => setPage(n)}
                      />
                    ),
                  )}
                  <SahifaTugma
                    belgi={<ChevronRight size={16} aria-hidden />}
                    nomi="Keyingi sahifa"
                    ochiq={page >= sahifalar}
                    bosildi={() => setPage((v) => Math.min(sahifalar, v + 1))}
                  />
                </nav>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/* ── Bo'laklar ─────────────────────────────────────────────────────── */

function SanaMaydon({
  id,
  nomi,
  qiymat,
  ozgardi,
  xatoli,
  uslub,
}: {
  id: string;
  nomi: string;
  qiymat: string;
  ozgardi: (v: string) => void;
  xatoli: boolean;
  uslub: React.CSSProperties;
}) {
  return (
    <div className="w-[180px] min-w-[150px] flex-shrink-0">
      <label
        htmlFor={id}
        className="mb-[6px] block text-[12px] leading-4"
        style={{ color: OCH_KUL }}
      >
        {nomi}
      </label>
      <div className="relative">
        <CalendarDays
          size={15}
          color={xatoli ? QIZIL : qiymat ? KO_K : OCH_KUL}
          aria-hidden
          className="pointer-events-none absolute left-[11px] top-1/2 z-[1] -translate-y-1/2"
        />
        {/* Brauzerning o'z kalendar belgisi shaffof qilinib butun maydonni
            qoplaydi: chapdagi ikonka Figma'dagidek qoladi, bosilganda esa
            baribir tizim kalendari ochiladi. */}
        <input
          id={id}
          type="date"
          value={qiymat}
          onChange={(e) => ozgardi(e.target.value)}
          aria-invalid={xatoli || undefined}
          className={`h-10 w-full rounded-[10px] bg-white pl-[34px] pr-[12px] text-[13px] leading-[18px] outline-none ${FOKUS} [&::-webkit-calendar-picker-indicator]:absolute [&::-webkit-calendar-picker-indicator]:inset-0 [&::-webkit-calendar-picker-indicator]:h-full [&::-webkit-calendar-picker-indicator]:w-full [&::-webkit-calendar-picker-indicator]:cursor-pointer [&::-webkit-calendar-picker-indicator]:opacity-0`}
          style={uslub}
        />
      </div>
    </div>
  );
}

function Chip({
  matn,
  rang,
  fon,
  olib,
}: {
  matn: string;
  rang: string;
  fon: string;
  olib: () => void;
}) {
  return (
    <span
      className="inline-flex h-[27px] max-w-full items-center gap-[7px] rounded-full pl-[11px] pr-[7px]"
      style={{ background: fon }}
    >
      <span className="truncate text-[12px] font-medium leading-4" style={{ color: rang }}>
        {matn}
      </span>
      <button
        type="button"
        onClick={olib}
        aria-label={`Filtrni olib tashlash: ${matn}`}
        className={`grid h-[17px] w-[17px] shrink-0 place-items-center rounded-full transition-opacity hover:opacity-60 ${FOKUS}`}
      >
        <X size={12} color={rang} aria-hidden />
      </button>
    </span>
  );
}

function Holat({
  ikon,
  sarlavha,
  sarlavhaRang,
  tavsif,
  amal,
}: {
  ikon: React.ReactNode;
  sarlavha: string;
  sarlavhaRang: string;
  tavsif: string;
  amal: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-[10px] text-center">
      {ikon}
      <div
        className="text-[16px] font-semibold leading-[22px]"
        style={{ color: sarlavhaRang }}
      >
        {sarlavha}
      </div>
      <p className="max-w-[420px] text-[13px] leading-[18px]" style={{ color: OCH_KUL }}>
        {tavsif}
      </p>
      {amal && <div className="mt-[4px]">{amal}</div>}
    </div>
  );
}

function SahifaTugma({
  belgi,
  nomi,
  faol,
  ochiq,
  bosildi,
}: {
  belgi: React.ReactNode;
  nomi: string;
  faol?: boolean;
  ochiq?: boolean;
  bosildi: () => void;
}) {
  return (
    <button
      type="button"
      onClick={bosildi}
      disabled={ochiq}
      aria-label={nomi}
      aria-current={faol ? "page" : undefined}
      className={`grid h-[34px] min-w-[34px] place-items-center rounded-[9px] px-[6px] text-[13px] font-medium leading-[18px] transition-colors ${
        ochiq ? "cursor-not-allowed" : faol ? "" : "hover:bg-[#f4f6fc]"
      } ${FOKUS}`}
      style={
        faol
          ? { background: KO_K, color: "#fff" }
          : {
              background: "#fff",
              color: ochiq ? XIRA : KUL,
              boxShadow: `inset 0 0 0 1px ${ochiq ? HOSHIYA_OCH : HOSHIYA_QUYUQ}`,
            }
      }
    >
      {belgi}
    </button>
  );
}

function Qator({ a, juft }: { a: AdminAudit; juft: boolean }) {
  const meta = AMAL[a.action];
  const guruh = meta ? GURUH[meta.guruh] : null;
  const d = new Date(a.createdAt);
  const yaroqli = !Number.isNaN(d.getTime());
  const nom = (a.adminName || "").trim();

  return (
    <tr
      className="border-b last:border-b-0"
      style={{ borderColor: HOSHIYA, background: juft ? ZEBRA : "#fff" }}
    >
      {/* Vaqt */}
      <td className="h-[61px] pl-5 pr-3 align-middle">
        {yaroqli ? (
          <>
            <div className="text-[14px] font-bold leading-[19px]" style={{ color: IK }}>
              {soat(d)}
            </div>
            <div className="text-[12px] leading-4" style={{ color: OCH_KUL }}>
              {sana(d)}
            </div>
          </>
        ) : (
          <span style={{ color: XIRA }}>—</span>
        )}
      </td>

      {/* Admin */}
      <td className="px-3 align-middle">
        <div className="flex items-center gap-[9px]">
          <span
            aria-hidden
            className="grid h-[28px] w-[28px] shrink-0 place-items-center rounded-full text-[11px] font-semibold"
            style={{
              background: nom ? AVATAR_KO_K : AVATAR_FON,
              color: nom ? KO_K : XIRA_QUYUQ,
            }}
          >
            {nom ? boshHarflar(nom) : "?"}
          </span>
          {/* Adminni o'chirish jurnal yozuvini o'chirmaydi — nomi topilmasa
              ham qator o'z joyida qoladi. */}
          <span
            className="truncate text-[13px] font-medium leading-[18px]"
            style={{ color: nom ? IK : XIRA_QUYUQ }}
            title={nom || undefined}
          >
            {nom || "— (noma'lum)"}
          </span>
        </div>
      </td>

      {/* Amal */}
      <td className="px-3 align-middle">
        <span
          className="inline-flex h-6 max-w-full items-center gap-[6px] rounded-lg px-[10px]"
          style={{ background: guruh ? guruh.fon : AVATAR_FON }}
          title={guruh ? `${guruh.nomi} · ${a.action}` : a.action}
        >
          <span
            aria-hidden
            className="h-[6px] w-[6px] shrink-0 rounded-full"
            style={{ background: guruh ? guruh.rang : XIRA_QUYUQ }}
          />
          {/* Katalogda yo'q kod — backend yangi amal qo'shgan. Qatorni
              yashirmaymiz, xom kodni ko'rsatamiz. */}
          <span
            className="truncate text-[12px] font-medium leading-4"
            style={{ color: guruh ? guruh.rang : XIRA_QUYUQ }}
          >
            {meta ? meta.yorliq : a.action}
          </span>
        </span>
      </td>

      {/* Maqsad */}
      <td className="px-3 align-middle">
        <Maqsad a={a} meta={meta} />
      </td>

      {/* Tafsilot */}
      <td className="px-3 pr-5 align-middle">
        <span
          className="block truncate text-[13px] leading-[18px]"
          style={{ color: a.detail ? OCH_KUL : XIRA }}
          title={a.detail || undefined}
        >
          {a.detail || "—"}
        </span>
      </td>
    </tr>
  );
}

function Maqsad({ a, meta }: { a: AdminAudit; meta?: AmalMeta }) {
  const tur = meta?.maqsad ?? "yoq";
  const t = (a.target || "").trim();
  if (tur === "yoq" || !t) return <span style={{ color: XIRA }}>—</span>;

  const { yorliq, xom } = MAQSAD[tur];
  const nom = (a.targetName || "").trim();

  // Xom nishon (login, sarlavha, fayl kaliti) — o'zi. Aks holda: server
  // topgan nom, topolmasa — qisqartirilgan id.
  let qiymat: string;
  let topildi: boolean;
  if (xom) {
    qiymat = tur === "login" && !t.startsWith("@") ? `@${t}` : t;
    topildi = true;
  } else if (nom) {
    qiymat = nom;
    topildi = true;
  } else {
    qiymat = `…${t.slice(-6)}`;
    topildi = false;
  }

  return (
    <div className="min-w-0">
      {yorliq && (
        <div className="text-[11px] leading-[15px]" style={{ color: XIRA_QUYUQ }}>
          {yorliq}
        </div>
      )}
      <div
        className="truncate text-[13px] leading-[18px]"
        style={{ color: topildi ? IK : OCH_KUL, fontWeight: topildi ? 500 : 400 }}
        title={t}
      >
        {qiymat}
      </div>
    </div>
  );
}
