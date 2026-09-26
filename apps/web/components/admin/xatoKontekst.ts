/**
 * "3.12 · Xatoliklar" — AI uchun kontekst matnining MAHALLIY zaxirasi.
 *
 * # NEGA BU FAYL BOR
 *
 * Asosiy manba — server: `GET /api/admin/errors/{id}/context` matnni
 * `internal/admin/errexport.go` da yig'adi va niqoblaydi. Lekin bu so'rov
 * yiqilsa yoki hali javob kelmagan bo'lsa, ekran bo'sh qolmasligi uchun
 * xuddi shu matn shu yerda, serverdan kelgan HAQIQIY `AdminErrorDetail`
 * ma'lumotidan yig'iladi. O'ylab topilgan hech narsa yo'q — faqat format.
 *
 * # NIQOBLASH
 *
 * `niqobla()` — serverdagi `errexport.go · maskSecrets` ning qisqartirilgan
 * nusxasi. HAQIQIY himoya serverda — matn tug'ilgan joyda niqoblanadi va
 * uni o'chiradigan parametr API'da umuman yo'q.
 */
import type {
  AdminErrorDetail,
  AdminErrorGroup,
  XatoKontekst,
  XatoKontekstKalit,
  XatoNamuna,
  XatoQadam,
  XatoQurilma,
} from "@/lib/api";
import { DARAJA, HOLAT, MODUL, p2 } from "./xato";

/* ── Niqoblash (serverdagi `maskSecrets` ning mahalliy nusxasi) ─────── */

const NIQOB = "‹niqoblangan›";

export function niqobla(s: string): string {
  if (!s) return "";
  return s
    .replace(/eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}(\.[A-Za-z0-9_-]+)?/g, NIQOB)
    .replace(/bearer\s+[A-Za-z0-9._~+/=-]{8,}/gi, `Bearer ${NIQOB}`)
    .replace(
      /\b(password|passwd|pwd|token|access_token|refresh|refresh_token|secret|apikey|api_key|authorization|otp|otp_code|totp)\b"?\s*[:=]\s*"?[^\s",;})\]]+/gi,
      `$1: ${NIQOB}`,
    )
    .replace(/\+?998[\s-]?\(?\d{2}\)?[\s-]?\d{3}[\s-]?\d{2}[\s-]?\d{2}/g, "+998 •• ••• •• ••")
    .replace(/\b(\d{1,3})\.(\d{1,3})\.\d{1,3}\.\d{1,3}\b/g, "$1.$2.•••.•••")
    .replace(/\b\d{9,}\b/g, "•••");
}

/* ── AI uchun kontekst (Figma 3.12.3 · L) ─────────────────────────────
   Matn serverdagi `internal/admin/errexport.go` bilan BIR XIL tartibda
   yig'iladi: sarlavha → tavsif qatorlari → bo'limlar. Server
   (`GET /api/admin/errors/{id}/context`) shu matnning o'zini qaytaradi,
   shuning uchun zaxiraga o'tilganda ham ekran o'zgarmaydi. */

const YOQ_QIY = "aniqlanmagan";

function qiy(v?: string): string {
  return v && v.trim() !== "" ? v : YOQ_QIY;
}

/** Bo'sh bo'laklarni tashlab qo'shadi: "Chrome 128 · " kabi dumaloq qolmaydi. */
function birik(sep: string, ...qismlar: (string | undefined)[]): string {
  return qismlar.map((q) => (q ?? "").trim()).filter(Boolean).join(sep);
}

function qavs(v?: string): string {
  return v && v.trim() !== "" ? `(${v})` : "";
}

function vaqtMatn(s?: string): string {
  if (!s) return YOQ_QIY;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return YOQ_QIY;
  return `${p2(d.getDate())}.${p2(d.getMonth() + 1)}.${d.getFullYear()} ${p2(d.getHours())}:${p2(d.getMinutes())}`;
}

/** `errlog.DeviceLabel` ning nusxasi. */
export function qurilmaYorliq(d?: XatoQurilma): string {
  if (!d) return "";
  const nom = birik(" ", d.brand, d.model) || (d.browser ?? "");
  const os = birik(" ", d.os, d.osVersion);
  if (nom && os) return `${nom} · ${os}`;
  return nom || os || d.platform || "";
}

/**
 * `errexport.deviceWithCode` ning nusxasi — yorliqqa model KODINI qo'shadi.
 *
 * NEGA kod yorliqning oxiriga emas, nom bo'lagining ortiga qo'yiladi:
 * yorliq ichida OS allaqachon bor ("nom · OS versiya"), kod esa apparat
 * variantini bildiradi va u nomga tegishli. Oxiriga qo'yilsa "… · Android 14
 * (2201117TG)" chiqib, kod OS versiyasiga tegishli bo'lib ko'rinardi.
 *
 * Nom umuman bo'lmaganda (brauzer yoki faqat platforma ma'lum) kod oxirida
 * qoladi — uni bog'laydigan boshqa bo'lak yo'q.
 */
function qurilmaKodBilan(d?: XatoQurilma): string {
  const yorliq = qurilmaYorliq(d);
  const kod = qavs(d?.modelCode);
  if (!yorliq || !kod) return yorliq;
  const nom = birik(" ", d?.brand, d?.model);
  if (nom && yorliq.startsWith(nom)) return `${nom} ${kod}${yorliq.slice(nom.length)}`;
  return `${yorliq} ${kod}`;
}

/**
 * `errexport.emulatorLabel` ning nusxasi: bitta maydon, ikki xil sarlavha.
 *
 * NEGA platformaga qarab: iOS hodisasida "Root" so'zi noto'g'ri — u yerda
 * tushuncha "Jailbreak" deb ataladi va mos kelmagan atama kontekstni
 * o'qiyotgan odamni ham, AI'ni ham chalg'itadi.
 */
function emulyatorNomi(platform?: string): string {
  return (platform ?? "").trim().toLowerCase() === "ios" ? "Simulyator · Jailbreak" : "Emulyator · Root";
}

const KOD_REF = /[\w./\\-]+\.(dart|go|ts|tsx|js|jsx|kt|swift):\d+/;

/** `errexport.codeCtxLines` — sarlavhadagi diapazonning yarim kengligi. */
const KOD_ATROF = 3;

/**
 * `errexport.codeTitle` ning nusxasi: "Tegishli kod (api_client.dart:289-295)".
 *
 * NEGA diapazon sarlavhada: AI'ga bitta qator emas, uning ATROFI kerak —
 * o'zgaruvchi yuqorida e'lon qilinadi, tekshiruv esa pastda bo'ladi. Manzil
 * topilmasa sarlavha o'zgarishsiz qoladi: taxminiy diapazon yozgandan ko'ra
 * hech narsa yozmaslik to'g'ri.
 */
function kodSarlavha(g: AdminErrorGroup, s?: XatoNamuna): string {
  const asos = "Tegishli kod";
  let ref = KOD_REF.exec(g.where ?? "")?.[0] ?? "";
  if (!ref) {
    // Stekning BIRINCHI kadri — xato tug'ilgan joy; keyingilari chaqiruvchilar.
    for (const qator of s?.stack ?? []) {
      const m = KOD_REF.exec(qator)?.[0];
      if (m) {
        ref = m;
        break;
      }
    }
  }
  const i = ref.lastIndexOf(":");
  if (i < 0) return asos;
  const n = Number.parseInt(ref.slice(i + 1), 10);
  if (!Number.isFinite(n) || n <= 0) return asos;
  // Fayl nomi qisqartiriladi (faqat oxirgi bo'lak): to'liq yo'l sarlavhani
  // ikki qatorga bo'lib tashlaydi, u esa bo'limning O'ZIDA to'liq turadi.
  let fayl = ref.slice(0, i);
  const j = Math.max(fayl.lastIndexOf("/"), fayl.lastIndexOf("\\"));
  if (j >= 0) fayl = fayl.slice(j + 1);
  if (!fayl) return asos;
  const dan = Math.max(1, n - KOD_ATROF);
  return `${asos} (${fayl}:${dan}-${n + KOD_ATROF})`;
}

function qadamNomi(k: XatoQadam["kind"]): string {
  const m: Record<XatoQadam["kind"], string> = {
    nav: "navigatsiya",
    screen: "ekran",
    action: "amal",
    request: "so'rov",
    response: "javob",
    crash: "CRASH",
  };
  return m[k] ?? k;
}

type Bolim = { title: string; lines: string[] };

function bolimlar(d: AdminErrorDetail, yoq: Set<XatoKontekstKalit>): Bolim[] {
  const g = d.group;
  const s = d.sample;
  const dev = s?.device;
  const out: Bolim[] = [];

  if (yoq.has("device")) {
    // Qurilma va OS BITTA qatorda (serverdagi `deviceLines` bilan bir xil):
    // yorliq ichida OS allaqachon bor, ortidan yana "OS: …" yozilsa AI'ga
    // bir xil ma'lumot ikki marta borardi va belgi/token sanog'i behuda
    // oshardi. Tekshiruv yorliqning MAZMUNIDA: takror rostdan bo'lganda
    // qator tushadi, brauzer yoki faqat platforma ma'lum bo'lgan hodisada
    // esa "OS:" qatori joyida qoladi.
    const osMatn = birik(" ", dev?.os, dev?.osVersion);
    const api = dev?.apiLevel ? `(API ${dev.apiLevel})` : "";
    const qurilma = qurilmaKodBilan(dev);
    const l = [`Ilova: ${qiy(birik(" ", dev?.appVersion, qavs(dev?.build)))}`];
    if (osMatn && qurilma.includes(osMatn)) {
      l.push(`Qurilma: ${birik(" ", qurilma, api)}`);
    } else {
      l.push(`Qurilma: ${qiy(qurilma)}`, `OS: ${qiy(birik(" ", osMatn, api))}`);
    }
    if (dev?.browser || dev?.platform === "web") l.push(`Brauzer: ${qiy(birik(" · ", dev?.browser, dev?.engine))}`);
    if (dev?.flutter || dev?.dart) l.push(`Flutter/Dart: ${qiy(birik(" / ", dev?.flutter, dev?.dart))}`);
    // Ekran va RAM bitta qatorda: ikkisi ham "qurilma qanchalik kuchsiz"
    // degan bitta savolga javob beradi va OOM yoki rasm hajmi bilan
    // bog'liq xatolikda faqat yonma-yon turganda ma'no chiqaradi.
    let ekran = `Ekran: ${qiy(dev?.screen)}`;
    const ram = (dev?.ram ?? "").trim();
    if (ram) ekran += ` · RAM ${ram}`;
    l.push(ekran);
    // Quyidagi maydonlar SHARTLI. Ular faqat mijoz `X-Client-Device`
    // sarlavhasini yuborganda ma'lum bo'ladi, ya'ni ko'pincha bo'sh.
    // Bo'shini "aniqlanmagan" deb yozish kontekstni uzaytirardi, AI'ga esa
    // hech narsa bermasdi — shu sababli `qiy()` emas, qatorning O'ZI
    // tashlab yuboriladi.
    const xotira = (dev?.storage ?? "").trim();
    if (xotira) l.push(`Xotira: ${xotira}`);
    l.push(`Tarmoq: ${qiy(dev?.network)}`, `Til: ${qiy(dev?.locale)}`);
    const batareya = (dev?.battery ?? "").trim();
    if (batareya) l.push(`Batareya: ${batareya}`);
    const emul = (dev?.emulator ?? "").trim();
    if (emul) l.push(`${emulyatorNomi(dev?.platform)}: ${emul}`);
    const yonalish = (dev?.orientation ?? "").trim();
    if (yonalish) l.push(`Orientatsiya: ${yonalish}`);
    // Server qatori: host · muhit · backend build. Host BIRINCHI turadi,
    // chunki bir necha nusxa ishlaganda "xatolik hammasidami yoki
    // bittasidami" savoliga faqat mashina nomi javob beradi; APP_ENV va
    // build esa "qaysi kod ishlagan" savolini yopadi.
    l.push(`Server: ${qiy(d.env.server)} · APP_ENV=${qiy(d.env.appEnv)} · backend build: ${qiy(d.env.version)}`);
    if (g.plannedVersion) l.push(`Rejalashtirilgan tuzatish versiyasi: ${g.plannedVersion}`);
    if (g.fixedVersion) l.push(`Avval tuzatilgan versiya: ${g.fixedVersion}`);
    out.push({ title: "Muhit", lines: l });
  }

  if (yoq.has("stack")) {
    const l: string[] = [];
    const xabar = niqobla(g.message || s?.message || "");
    if (xabar) l.push(xabar);
    if (s?.stack?.length) l.push(...s.stack.map(niqobla));
    out.push({ title: "Xato matni va stack trace", lines: l.length ? l : [YOQ_QIY] });
  }

  if (yoq.has("code")) {
    const korilgan = new Set<string>();
    const l: string[] = [];
    const qosh = (v?: string) => {
      const t = (v ?? "").trim();
      if (!t || korilgan.has(t)) return;
      korilgan.add(t);
      l.push(t);
    };
    qosh(g.where);
    for (const qator of s?.stack ?? []) {
      if (l.length >= 4) break;
      qosh(KOD_REF.exec(qator)?.[0]);
    }
    if (l.length === 0) l.push(YOQ_QIY);
    else l.push("(fayl mazmuni serverda saqlanmaydi — yuqoridagi manzillarni repozitoriydan oching)");
    out.push({ title: kodSarlavha(g, s), lines: l });
  }

  if (yoq.has("request")) {
    let bosh = birik(" ", s?.method, s?.path || g.path || YOQ_QIY);
    if (s?.status) bosh += ` → ${s.status}`;
    if (s?.durationMs) bosh += ` · ${s.durationMs} ms`;
    out.push({
      title: "So'rov",
      lines: [
        qiy(bosh),
        `request_id: ${qiy(s?.requestId)}`,
        `guruh ID: ${g.ref} · fingerprint: ${g.fingerprint.slice(0, 12)}`,
        "IP va to'liq telefon yig'ilmaydi (jurnalda ham yo'q)",
      ],
    });
  }

  if (yoq.has("steps")) {
    const l = (s?.steps ?? []).map((q) => {
      const t = new Date(q.at);
      return `${p2(t.getHours())}:${p2(t.getMinutes())}:${p2(t.getSeconds())} ${qadamNomi(q.kind)} ${niqobla(q.text)}`;
    });
    out.push({
      title: "Xatolikdan oldingi qadamlar",
      lines: l.length ? l : ["qadamlar yozib olinmagan (mijoz breadcrumb yubormagan)"],
    });
  }

  out.push({
    title: "Kutilgan xulq",
    lines: [
      "So'rov muvaffaqiyatli bajarilishi yoki xato foydalanuvchiga tushunarli xabar bilan qaytishi kerak edi; ilova ishdan chiqmasligi kerak.",
    ],
  });
  out.push({
    title: "Savol",
    lines: [
      "Shu xatolikning sababi nimada va uni qanday tuzatish kerak? Tuzatishni qaysi faylning qaysi qatorida qilish kerakligini ko'rsat.",
    ],
  });
  return out;
}

/** Qaysi kalitlar hozir mavjud emas (dizaynda kulrang ko'rsatilgan). */
export const KONTEKST_YOQ: XatoKontekstKalit[] = ["serverlog", "similar"];

/**
 * Kontekst matnini serverdan kelgan haqiqiy `AdminErrorDetail` dan mahalliy
 * yig'adi. `GET /api/admin/errors/{id}/context` javob bermaganda zaxira
 * sifatida ishlatiladi; tartib va format server bilan bir xil.
 */
export function kontekstYasash(
  d: AdminErrorDetail,
  format: "md" | "json" | "txt",
  include: XatoKontekstKalit[],
): XatoKontekst {
  const g = d.group;
  const bor = include.filter((k) => !KONTEKST_YOQ.includes(k));
  const yoq = include.filter((k) => KONTEKST_YOQ.includes(k));
  const head = `Xatolik: ${g.code} (${g.ref})`;
  const subtitle = [
    birik(" · ", `Muhimlik: ${DARAJA[g.severity]?.nomi ?? g.severity}`, `Holat: ${HOLAT[g.status]?.nomi ?? g.status}`, g.assignee ? `Mas'ul: ${g.assignee}` : ""),
    `Birinchi: ${vaqtMatn(g.firstSeenAt)} · Oxirgi: ${vaqtMatn(g.lastSeenAt)} · ${g.count} hodisa / ${g.usersCount} foydalanuvchi`,
  ];
  if (g.title) subtitle.push(`Sarlavha: ${niqobla(g.title)}`);
  if (g.runtime || g.module) subtitle.push(`Manba: ${birik(" · ", g.runtime, MODUL[g.module] ?? g.module)}`);

  const sections = bolimlar(d, new Set(bor));
  let text: string;
  if (format === "json") {
    text = JSON.stringify({ head, subtitle, sections }, null, 2);
  } else {
    const md = format === "md";
    const q: string[] = [md ? `# ${head}` : head.toUpperCase(), ...subtitle];
    for (const s of sections) {
      q.push("", md ? `## ${s.title}` : s.title.toUpperCase(), ...s.lines);
    }
    text = `${q.join("\n")}\n`;
  }
  const chars = Array.from(text).length;
  return {
    format,
    text,
    chars,
    tokens: Math.floor((chars + 3) / 4),
    masked: true,
    include: bor,
    unavailable: yoq,
    filename: `${g.ref}-${g.code}.${format}`,
  };
}
