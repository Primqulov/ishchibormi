"use client";
/**
 * "3.12.1 · Xatolik — batafsil (1440 × 1400)" — ro'yxatdagi bitta guruh
 * bosilganda ochiladigan ekran.
 *
 * # NIMA UCHUN BU EKRAN BOR
 *
 * Ro'yxat "nima bo'ldi va necha marta" degan savolga javob beradi. Bu
 * ekran esa "QACHON, KIMDA, QAYSI QURILMADA va NIMA QILINGANDA" degan
 * savolga: hodisa vaqti soniyasigacha, 24 soatlik chizig'i, so'nggi
 * hodisalar jadvali, stack trace va butun amallar tarixi.
 *
 * # MA'LUMOT QAYERDAN
 *
 * Faqat `GET /api/admin/errors/{id}` — o'ylab topilgan zaxira ma'lumot
 * YO'Q. So'rov yiqilsa ekran xatoni ochiq ko'rsatadi: diagnostika
 * ekranida soxta raqamni haqiqiy deb o'qigan admin noto'g'ri qaror
 * qabul qiladi.
 *
 * # XAVFSIZLIK
 *
 * · Sahifa `robots: noindex` (server qobig'ida) — stack trace va endpoint
 *   nomlari qidiruvga tushmasligi kerak.
 * · Telefon raqamlari SERVERDA niqoblanadi, niqobni ochadigan endpoint
 *   panelda umuman yo'q.
 * · Holatni o'zgartirish, mas'ul biriktirish va e'tiborsiz qoldirish —
 *   faqat superadmin (`RequireRole("superadmin")`), "e'tiborsiz" esa
 *   majburiy sabab va ikki bosqichli tasdiq bilan.
 * · Telegram — TASHQARIGA chiqadigan amal: tasdiq oynasi va 60 soniyalik
 *   sovish oynasi (server ham `tgCooldown` bilan shuni talab qiladi).
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  ChevronDown,
  CircleAlert,
  EyeOff,
  Send,
  ShieldOff,
  TriangleAlert,
} from "lucide-react";
import {
  APIError,
  AdminErrorDetail,
  AdminErrorGroup,
  AdminRole,
  XatoHolat,
  XatoKontekstKalit,
  XatoMasul,
  XatoQolHolat,
  api,
  getAdminRole,
} from "@/lib/api";
import { AdminModal } from "@/components/admin/AdminModal";
import {
  HOSHIYA,
  HOSHIYA_OCH,
  HOSHIYA_QUYUQ,
  IK,
  KO_K,
  KUL,
  OCH_KUL,
  QIZIL,
  QIZIL_HOSHIYA,
  SOYA,
  XIRA_QUYUQ,
  tugma,
} from "@/components/admin/ui";
import {
  Chip,
  FOKUS,
  Izohcha,
  Nishon,
  Xabarlar,
  nusxaOl,
  useXabarlar,
} from "@/components/admin/xatoQismlar";
import {
  DARAJA,
  HOLAT,
  YOQ,
  kunNuqta,
  kunSoat,
  son,
} from "@/components/admin/xato";
import { niqobla } from "@/components/admin/xatoKontekst";
import AiKontekst, { KONTEKST_SUKUT } from "./AiKontekst";
import AiTahlil from "./AiTahlil";
import {
  ChiziqKarta,
  HodisalarKarta,
  HolatKarta,
  MuhitKarta,
  OdamlarKarta,
  QadamlarKarta,
  SorovKarta,
  StackKarta,
  TarixKarta,
  TasirKarta,
  xatoTuri,
} from "./XatoKartalar";

/** Telegram sovish oynasi — server `tgCooldown` bilan bir xil qiymat. */
const TG_SOVISH = 60_000;

/**
 * "E'tiborsiz qoldirish" sababining eng kichik uzunligi.
 *
 * Server `minIgnoreReason = 10` (`internal/admin/errors.go`) va qisqasini
 * 400 `reason_required` bilan qaytaradi — tekshiruvni shu yerda ham
 * takrorlaymiz, aks holda admin yozganini yo'qotib, xatoni faqat
 * yuborgandan keyin ko'rardi.
 */
const SABAB_ENG_KAM = 10;

/**
 * Versiya satrining eng katta uzunligi.
 *
 * Server `maxVersionLen = 40` (`internal/admin/errors.go`) va uzunini
 * JIMGINA kesib tashlaydi — 400 qaytarmaydi. Ya'ni chegara mijozda
 * bo'lmasa, admin "1.4.3 (121) — hotfix …" deb yozgan matnining yarmi
 * yo'qolgani faqat sahifa qayta yuklangach ma'lum bo'lardi.
 */
const VERSIYA_ENG_KOP = 40;

/** "hozir" ni yangilash oralig'i: nisbiy vaqtlar qotib qolmasin. */
const SOAT_YANGILASH = 60_000;

type OchiqOyna =
  | { tur: "holat"; holat: XatoQolHolat }
  | { tur: "masul" }
  | { tur: "izoh" }
  | { tur: "telegram" }
  | null;

export default function XatolikBatafsil() {
  const parametr = useParams<{ id: string | string[] }>();
  const id = useMemo(() => {
    const v = parametr?.id;
    const s = Array.isArray(v) ? v[0] : v;
    return typeof s === "string" ? decodeURIComponent(s) : "";
  }, [parametr]);

  const [d, setD] = useState<AdminErrorDetail | null>(null);
  const [yuklanmoqda, setYuklanmoqda] = useState(true);
  const [xato, setXato] = useState<APIError | null>(null);
  const [rol, setRol] = useState<AdminRole | null>(null);
  /**
   * `hozir` DOIM `useEffect` da o'rnatiladi.
   *
   * Serverda `Date.now()` boshqa qiymat beradi va "Bugun, 14:21" yozuvi
   * gidratsiyada mos kelmay React ogohlantirishini chiqarardi. Shu sababli
   * boshlang'ich qiymat `null` va vaqt ko'rsatadigan hamma narsa faqat
   * klientda chiziladi.
   */
  const [hozir, setHozir] = useState<number | null>(null);

  const { xabarlar, xabarQosh, xabarYop } = useXabarlar();

  const [oyna, setOyna] = useState<OchiqOyna>(null);
  const [matn, setMatn] = useState(""); // izoh / sabab maydoni
  const [matnXato, setMatnXato] = useState(false);
  const [tasdiq, setTasdiq] = useState(false);
  const [saqlanmoqda, setSaqlanmoqda] = useState(false);
  const [masul, setMasul] = useState<string>("");

  /**
   * Holat oynasining SHARTLI maydonlari (Figma 3.12.3 · J).
   *
   * `holatMasul` mas'ul tanlash oynasidagi `masul` dan ATAYLAB ajratilgan:
   * bittasi "kim javobgar" degan mustaqil amal (`PATCH /assignee`),
   * ikkinchisi esa "ishni boshladim" o'tishining bir qismi. Bitta holatni
   * ikkalasiga ulasak, holat oynasini bekor qilish mas'ul oynasidagi
   * tanlovni ham buzardi.
   *
   * `versiya` bitta: oynada bir vaqtda YO "rejalashtirilgan", YO
   * "tuzatilgan" maydoni ko'rinadi, ikkalasi hech qachon birga emas.
   */
  const [holatMasul, setHolatMasul] = useState<string>("");
  const [versiya, setVersiya] = useState("");

  const [tgVaqt, setTgVaqt] = useState<number | null>(null);
  const [qoldi, setQoldi] = useState(0);

  /**
   * Kontekst bo'limlari tanlovi — IKKI karta uchun bitta manba.
   *
   * Pastdagi «Nimalar qo'shilsin» kataklari nafaqat ko'rinadigan matnni,
   * balki AI TAHLILI ko'radigan matnni ham boshqaradi. Ikki joyda ikkita
   * holat bo'lsa, admin stack trace'ni o'chirib qo'yib, AI baribir uni
   * o'qiyotganini bilmasdi.
   */
  const [bolaklar, setBolaklar] = useState<XatoKontekstKalit[]>(KONTEKST_SUKUT);

  /** Mas'ul tanlash ro'yxati — `GET /api/admin/errors/assignees`. */
  const [masullar, setMasullar] = useState<XatoMasul[]>([]);

  useEffect(() => {
    setHozir(Date.now());
    setRol(getAdminRole() as AdminRole | null);
    const t = window.setInterval(() => setHozir(Date.now()), SOAT_YANGILASH);
    return () => window.clearInterval(t);
  }, []);

  // Telegram sovish oynasi — faqat u faol bo'lganda taymer ishlaydi.
  useEffect(() => {
    if (tgVaqt === null) return;
    const hisobla = () => {
      const q = Math.max(0, Math.ceil((tgVaqt + TG_SOVISH - Date.now()) / 1000));
      setQoldi(q);
      if (q === 0) setTgVaqt(null);
    };
    hisobla();
    const t = window.setInterval(hisobla, 1000);
    return () => window.clearInterval(t);
  }, [tgVaqt]);

  /**
   * Sovish oynasini SERVER ma'lumotidan boshlaymiz.
   *
   * Aks holda sahifani yangilash sovish oynasini nolga qaytarardi va
   * xabarni ketma-ket yuborish mumkin bo'lardi (server baribir rad
   * qiladi, lekin admin buni faqat xatoda bilib olardi).
   */
  const tgVaqti = d?.group.tgSentAt;
  useEffect(() => {
    if (!tgVaqti) return;
    const t = new Date(tgVaqti).getTime();
    if (Number.isNaN(t) || Date.now() - t >= TG_SOVISH) return;
    setTgVaqt(t);
  }, [tgVaqti]);

  /* ── Yuklash ───────────────────────────────────────────────────── */

  const soravRaqami = useRef(0);

  const yukla = useCallback(
    async (jim = false) => {
      if (!id) {
        setYuklanmoqda(false);
        setXato({ code: "request_failed", message: "Xatolik manzili noto'g'ri." } as APIError);
        return;
      }
      const men = ++soravRaqami.current;
      if (!jim) setYuklanmoqda(true);
      try {
        const res = await api.get<AdminErrorDetail>(
          `/api/admin/errors/${encodeURIComponent(id)}`,
          { auth: "admin" } as any,
        );
        if (men !== soravRaqami.current) return;
        setD(res);
        setXato(null);
      } catch (e) {
        if (men !== soravRaqami.current) return;
        setD(null);
        setXato((e as APIError) ?? null);
      } finally {
        if (men === soravRaqami.current) setYuklanmoqda(false);
      }
    },
    [id],
  );

  useEffect(() => {
    yukla();
  }, [yukla]);

  /**
   * Mas'ullar ro'yxati bir marta yuklanadi.
   *
   * `GET /admins` ATAYLAB ishlatilmaydi: u superadmin darajasida va butun
   * kadr hisobini beradi. Bu endpoint esa tor proyeksiya — faqat faol
   * hisoblarning id, nomi va roli.
   */
  useEffect(() => {
    let bekor = false;
    (async () => {
      try {
        const res = await api.get<{ items: XatoMasul[] }>("/api/admin/errors/assignees", {
          auth: "admin",
        } as any);
        if (!bekor) setMasullar(res.items ?? []);
      } catch {
        // Ro'yxat kelmasa bo'sh qoladi — o'ylab topilgan ism KO'RSATILMAYDI.
        // Oynalarda "Men olaman" / "Biriktirilmasin" varianti baribir bor.
        if (!bekor) setMasullar([]);
      }
    })();
    return () => {
      bekor = true;
    };
  }, []);

  /**
   * Yorliqdan mas'ulning id'sini topish.
   *
   * Guruh JSON'ida faqat YORLIQ bor ("Sardor Rasulov · admin"), `assigneeId`
   * ataylab yopiq. Shuning uchun oynani ochishda joriy mas'ulni tanlangan
   * qilib ko'rsatishning yagona yo'li — nom bo'yicha solishtirish. Solishtirish
   * `startsWith` bilan: yorliqqa rol qo'shilgan, ro'yxatda esa faqat nom.
   */
  const masulId = useCallback(
    (yorliq?: string) => masullar.find((m) => yorliq?.startsWith(m.label))?.id ?? "",
    [masullar],
  );

  /* ── Ruxsat ────────────────────────────────────────────────────── */

  /**
   * Rollar SERVERDAGI qoida bilan bir xil (`internal/admin/errors.go`):
   *
   * · support — sahifa umuman yo'q (`RequireRole("moderator")`).
   * · moderator — ko'radi, holatni yuritadi, izoh yozadi, mas'ul
   *   biriktiradi. Kuzatish va tuzatish oqimi kundalik ish.
   * · superadmin — qo'shimcha ravishda "E'tiborsiz qoldirish"
   *   (`PatchErrorStatus` handler ichida yana bir marta tekshiradi).
   *
   * Rol hali o'qilmagan bo'lsa (`null`) tugmalarni bloklamaymiz: haqiqiy
   * qaror baribir serverda, panel faqat aniq man etilganini yashiradi.
   */
  const ruxsatYoq = xato?.code === "forbidden" || rol === "support";
  const ozgartirsaBoladi = rol !== "support";
  const etiborsizQila = rol !== "moderator" && rol !== "support";

  const g = d?.group ?? null;

  /* ── O'zgarishlarni saqlash ────────────────────────────────────────
     Holat, izoh, mas'ul va Telegram — to'rttasi ham o'z endpointiga
     boradi va javobda YANGILANGAN guruhni qaytaradi. So'rov yiqilsa
     o'zgarish ekranda QO'LLANMAYDI va xabarchada xato ochiq aytiladi:
     yolg'on muvaffaqiyat eng yomon variant. */

  /** Server qaytargan guruh bilan almashtirish (tarix ham yangilanadi). */
  const guruhYangila = useCallback((gr: AdminErrorGroup) => {
    setD((oldin) => (oldin ? { ...oldin, group: gr } : oldin));
  }, []);

  /* ── Amallar ───────────────────────────────────────────────────── */

  const oynaYop = useCallback(() => {
    if (saqlanmoqda) return;
    setOyna(null);
    setMatn("");
    setMatnXato(false);
    setTasdiq(false);
    // Versiya va mas'ul ham tozalanadi: "Bartaraf etilmoqda" oynasida
    // yozilgan "1.4.3" bekor qilingandan keyin "Bartaraf etildi" oynasida
    // qolib ketsa, admin bilmagan holda REJANI tuzatilgan versiya deb
    // yuborardi.
    setVersiya("");
    setHolatMasul("");
  }, [saqlanmoqda]);

  const holatOch = useCallback(
    (h: XatoHolat) => {
      if (!ozgartirsaBoladi || !g || h === g.status) return;
      // `regressed` — tizim belgisi, qo'lda qo'yilmaydi (server 400 qaytaradi).
      if (h === "regressed") return;
      if (h === "ignored" && !etiborsizQila) {
        xabarQosh(
          "xato",
          "Ruxsat yetarli emas",
          "E'tiborsiz qoldirish faqat superadmin uchun — guruh hisobotlardan butunlay chiqib ketadi.",
        );
        return;
      }
      setMatn(h === "ignored" ? "" : (g.note ?? ""));
      setMatnXato(false);
      setTasdiq(false);
      // Maydonlar MAVJUD qiymat bilan ochiladi: "Bartaraf etilmoqda" ni
      // qayta saqlash (izohni tuzatish) rejalashtirilgan versiyani
      // o'chirib yubormasligi kerak. "Bartaraf etildi" da esa reja
      // TAKLIF sifatida qo'yiladi — build raqami odatda unga qo'shiladi
      // ("1.4.3" → "1.4.3 (121)"), noldan terish esa xato terishga olib
      // kelardi.
      setVersiya(h === "fixing" ? (g.plannedVersion ?? "") : h === "resolved" ? (g.fixedVersion ?? g.plannedVersion ?? "") : "");
      setHolatMasul(h === "fixing" ? masulId(g.assignee) : "");
      setOyna({ tur: "holat", holat: h });
    },
    [ozgartirsaBoladi, etiborsizQila, g, masulId, xabarQosh],
  );

  const holatSaqla = useCallback(async () => {
    if (!g || !oyna || oyna.tur !== "holat") return;
    const yangi = oyna.holat;
    const izoh = matn.trim();

    // "E'tiborsiz" — hisobotlardan chiqarib tashlaydigan amal: sabab
    // MAJBURIY va ikkinchi bosqichda yana bir marta so'raladi. Uzunlik
    // chegarasi serverdagi `minIgnoreReason` bilan bir xil: qisqa sababni
    // server 400 `reason_required` bilan qaytarardi.
    if (yangi === "ignored") {
      if (izoh.length < SABAB_ENG_KAM) {
        setMatnXato(true);
        return;
      }
      if (!tasdiq) {
        setTasdiq(true);
        return;
      }
    }

    // Versiya oynada `maxLength` bilan cheklangan, lekin trim shart:
    // "1.4.3 " kabi bo'shliqli qiymat serverda boshqa satr sifatida
    // saqlanib, regressiya tekshiruvida mos kelmay qolardi.
    const v = versiya.trim().slice(0, VERSIYA_ENG_KOP);
    const reja = yangi === "fixing" ? v : "";
    const tuzatilgan = yangi === "resolved" ? v : "";
    const tanlanganMasul = yangi === "fixing" ? masullar.find((m) => m.id === holatMasul) : undefined;

    // Xabarcha sarlavhasida VERSIYA ham turadi ("Bartaraf etildi ·
    // 1.4.3 (121)"): admin yozgan raqamni darhol ko'rib, xato terilgan
    // bo'lsa o'sha zahoti tuzatsin — aks holda buni faqat kartadan,
    // sahifa yangilangandan keyin sezardi.
    const sarlavha = `Xatolik «${HOLAT[yangi].nomi}${v ? ` · ${v}` : ""}» deb belgilandi`;

    setSaqlanmoqda(true);
    try {
      // `note` va `reason` — SERVERDA ikki xil maydon: birinchisi ro'yxatda
      // ko'rinadigan izoh, ikkinchisi e'tiborsiz qoldirishning majburiy
      // sababi. Ikkalasini bitta maydonga yuborish 400 qaytaradi.
      const tana: Record<string, string> =
        yangi === "ignored" ? { status: yangi, reason: izoh } : { status: yangi, note: izoh };
      // Qo'shimcha maydonlar faqat TO'LDIRILGAN bo'lsa qo'shiladi.
      // Bo'sh `assigneeId` serverda "mas'ulni o'zgartirma" degani, bo'sh
      // versiya esa mavjud qiymatni saqlab qoladi — ya'ni bo'sh satr
      // yuborish zararsiz ko'rinsa-da, kelajakda "tozalash" ma'nosini
      // olishi mumkin. Kalit nomlari `errStatusReq` bilan AYNAN bir xil.
      if (tanlanganMasul) tana.assigneeId = tanlanganMasul.id;
      if (reja) tana.plannedVersion = reja;
      if (tuzatilgan) tana.fixedVersion = tuzatilgan;

      const gr = await api.patch<AdminErrorGroup>(
        `/api/admin/errors/${encodeURIComponent(g.id)}/status`,
        tana,
        { auth: "admin" } as any,
      );
      if (gr?.id) guruhYangila(gr);
      xabarQosh(
        "ok",
        sarlavha,
        tanlanganMasul
          ? `${g.ref} · mas'ul: ${tanlanganMasul.label}`
          : `${g.ref} · ${HOLAT[yangi].izoh}`,
      );
      setOyna(null);
      setMatn("");
      setVersiya("");
      setTasdiq(false);
      yukla(true);
    } catch (e) {
      const err = e as APIError | undefined;
      xabarQosh(
        "xato",
        "Amalni bajarib bo'lmadi",
        err?.code === "forbidden"
          ? "E'tiborsiz qoldirish faqat superadmin uchun."
          : err?.code === "reason_required"
            ? `Sabab kamida ${SABAB_ENG_KAM} belgi bo'lishi kerak.`
            : err?.message || "Server javob bermadi. Qayta urinib ko'ring.",
      );
      setTasdiq(false);
    } finally {
      setSaqlanmoqda(false);
    }
  }, [
    g,
    oyna,
    matn,
    versiya,
    holatMasul,
    masullar,
    tasdiq,
    xabarQosh,
    yukla,
    guruhYangila,
  ]);

  const izohSaqla = useCallback(async () => {
    if (!g) return;
    const t = matn.trim();
    if (t.length < 3) {
      setMatnXato(true);
      return;
    }
    setSaqlanmoqda(true);
    try {
      // Izoh HOLATNI o'zgartirmaydi — shuning uchun moderator ham yoza
      // oladi (`internal/admin/errors.go` · PostErrorNote).
      const gr = await api.post<AdminErrorGroup>(
        `/api/admin/errors/${encodeURIComponent(g.id)}/notes`,
        { text: t },
        { auth: "admin" } as any,
      );
      if (gr?.id) guruhYangila(gr);
      setOyna(null);
      setMatn("");
      xabarQosh("ok", "Izoh qo'shildi", `${g.ref} · amallar tarixiga yozildi.`);
    } catch (e) {
      const err = e as APIError | undefined;
      xabarQosh(
        "xato",
        "Izohni saqlab bo'lmadi",
        err?.message || "Server javob bermadi. Qayta urinib ko'ring.",
      );
    } finally {
      setSaqlanmoqda(false);
    }
  }, [g, matn, guruhYangila, xabarQosh]);

  const masulSaqla = useCallback(async () => {
    if (!g) return;
    const tanlangan = masullar.find((m) => m.id === masul);
    setSaqlanmoqda(true);
    try {
      // Bo'sh `assigneeId` — biriktirishni olib tashlash (server shunday
      // tushunadi va `assignee` maydonini o'chiradi).
      const gr = await api.patch<AdminErrorGroup>(
        `/api/admin/errors/${encodeURIComponent(g.id)}/assignee`,
        { assigneeId: masul },
        { auth: "admin" } as any,
      );
      if (gr?.id) guruhYangila(gr);
      setOyna(null);
      xabarQosh(
        "ok",
        tanlangan ? "Mas'ul biriktirildi" : "Mas'ul olib tashlandi",
        tanlangan ? `${g.ref} · ${tanlangan.label}` : `${g.ref} · endi mas'ul yo'q`,
      );
    } catch (e) {
      const err = e as APIError | undefined;
      xabarQosh(
        "xato",
        "Mas'ulni o'zgartirib bo'lmadi",
        err?.code === "bad_assignee"
          ? "Bu admin topilmadi yoki hisobi faol emas."
          : err?.message || "Server javob bermadi.",
      );
    } finally {
      setSaqlanmoqda(false);
    }
  }, [g, masul, masullar, guruhYangila, xabarQosh]);

  const tgYubor = useCallback(async () => {
    if (!g) return;
    setSaqlanmoqda(true);
    try {
      const gr = await api.post<AdminErrorGroup>(
        `/api/admin/errors/${encodeURIComponent(g.id)}/telegram`,
        {},
        { auth: "admin" } as any,
      );
      if (gr?.id) guruhYangila(gr);
      setOyna(null);
      setTgVaqt(Date.now());
      xabarQosh("ok", "Telegram'ga yuborildi", `${g.ref} · ogohlantirish kanaliga tushdi.`);
    } catch (e) {
      const err = e as APIError | undefined;
      // 429 — server sovish oynasini hisoblab beradi; taymerni o'sha
      // qiymatdan boshlaymiz, o'zimizniki bilan taxmin qilmaymiz.
      const qolgan = Number((err?.details as { retryAfter?: number } | undefined)?.retryAfter);
      if (err?.code === "cooldown" && Number.isFinite(qolgan)) {
        setTgVaqt(Date.now() - (TG_SOVISH - qolgan * 1000));
        setOyna(null);
        xabarQosh("xato", "Hozir yuborilgan", `${qolgan} soniyadan keyin qayta urinib ko'ring.`);
      } else if (err?.code === "tg_not_configured") {
        setOyna(null);
        xabarQosh(
          "xato",
          "Telegram sozlanmagan",
          "TELEGRAM_BOT_TOKEN va ERROR_ALERT_CHAT_ID berilmagan — kanalga yuborib bo'lmaydi.",
        );
      } else {
        xabarQosh(
          "xato",
          "Telegram'ga yuborib bo'lmadi",
          err?.message || "Server javob bermadi. Qayta urinib ko'ring.",
        );
      }
    } finally {
      setSaqlanmoqda(false);
    }
  }, [g, guruhYangila, xabarQosh]);

  /* ── Buferga nusxalash ─────────────────────────────────────────── */

  const nusxa = useCallback(
    async (sarlavha: string, tayyorla: () => string) => {
      // Niqoblashni yana bir marta qo'llaymiz: matn buferga, undan esa
      // istalgan joyga (chat, tiket, AI) tushadi.
      const ok = await nusxaOl(niqobla(tayyorla()));
      if (ok) xabarQosh("ok", sarlavha, "Matn buferga olindi.");
      else
        xabarQosh(
          "xato",
          "Nusxalab bo'lmadi",
          "Brauzer bufer ruxsatini bermadi. Matnni qo'lda belgilang.",
        );
    },
    [xabarQosh],
  );

  const stackNusxa = useCallback(() => {
    if (!d) return;
    const s = d.sample;
    nusxa("Stack trace nusxalandi", () =>
      [
        `${d.group.ref} · ${d.group.title}`,
        `${xatoTuri(d)} · ${d.group.code}`,
        s?.at ? kunSoat(new Date(s.at), hozir ?? Date.now()) : "",
        "",
        s?.message ?? "",
        ...(s?.stack ?? []),
      ]
        .filter((x) => x !== "")
        .join("\n"),
    );
  }, [d, hozir, nusxa]);

  const sorovNusxa = useCallback(() => {
    if (!d?.sample) return;
    const s = d.sample;
    nusxa("So'rov ma'lumotlari nusxalandi", () =>
      [
        `${d.group.ref} · ${d.group.title}`,
        `Metod va endpoint: ${[s.method, s.path].filter(Boolean).join(" ") || YOQ}`,
        `Javob kodi: ${s.status ?? YOQ}`,
        `requestId: ${s.requestId ?? YOQ}`,
        `Admin: ${[s.actor, s.actorRole].filter(Boolean).join(" · ") || YOQ}`,
        `Davomiylik: ${s.durationMs ? `${s.durationMs} ms` : YOQ}`,
      ].join("\n"),
    );
  }, [d, nusxa]);

  /* ── Ko'rinish ─────────────────────────────────────────────────── */

  const karta: React.CSSProperties = useMemo(
    () => ({ boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${SOYA}` }),
    [],
  );

  const dMeta = g ? DARAJA[g.severity] : null;
  const hMeta = g ? HOLAT[g.status] : null;

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {/* ══ Sahifa sarlavhasi (Figma 394:87) ═══════════════════════ */}
      <div
        className="flex min-h-[83px] min-w-0 flex-wrap items-center justify-between gap-3 rounded-[14px] bg-white px-5 py-[14px]"
        style={karta}
      >
        <div className="flex min-w-0 items-center gap-[14px]">
          <Link
            href="/admin/errors"
            aria-label="Xatoliklar ro'yxatiga qaytish"
            className={`grid h-[34px] w-[34px] shrink-0 place-items-center rounded-[10px] bg-white transition-colors hover:bg-[#f4f6fc] ${FOKUS}`}
            style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}` }}
          >
            <ArrowLeft size={17} color={IK} aria-hidden />
          </Link>
          <div className="min-w-0">
            <h1 className="truncate text-[22px] font-bold leading-7" style={{ color: IK }}>
              {g ? g.title || g.code : "Xatolik"}
            </h1>
            <div className="mt-[4px] flex min-w-0 flex-wrap items-center gap-[8px]">
              {g ? (
                <>
                  <Chip ipucha={g.code}>{g.code}</Chip>
                  <span className="min-w-0 truncate text-[12px] leading-4" style={{ color: OCH_KUL }}>
                    {[g.runtime, g.where].filter(Boolean).join(" · ")}
                    {g.runtime || g.where ? "  ·  " : ""}
                    Guruh ID: {g.ref}
                  </span>
                </>
              ) : (
                <span className="text-[12px] leading-4" style={{ color: XIRA_QUYUQ }}>
                  {yuklanmoqda ? "Yuklanmoqda…" : "Ma'lumot yo'q"}
                </span>
              )}
            </div>
          </div>
        </div>

        {g && (
          <div className="flex shrink-0 flex-wrap items-center gap-[10px]">
            <button
              type="button"
              onClick={() => holatOch("resolved")}
              disabled={!ozgartirsaBoladi || g.status === "resolved"}
              title={
                !ozgartirsaBoladi
                  ? "Holatni superadmin va moderator o'zgartira oladi."
                  : g.status === "resolved"
                    ? "Xatolik allaqachon bartaraf etilgan deb belgilangan."
                    : undefined
              }
              className={`${tugma("asosiy", { ochiq: !ozgartirsaBoladi || g.status === "resolved" }).className} ${FOKUS}`}
              style={{
                ...tugma("asosiy", { ochiq: !ozgartirsaBoladi || g.status === "resolved" }).style,
                height: 38,
                borderRadius: 9,
                paddingLeft: 16,
                paddingRight: 16,
              }}
            >
              Hal qilindi deb belgilash
            </button>
            <button
              type="button"
              onClick={() => holatOch("ignored")}
              disabled={!etiborsizQila || g.status === "ignored"}
              title={
                !etiborsizQila
                  ? "E'tiborsiz qoldirish faqat superadmin uchun."
                  : g.status === "ignored"
                    ? "Xatolik allaqachon e'tiborsiz qoldirilgan."
                    : "Sabab yozish majburiy — guruh hisobotlardan chiqariladi."
              }
              className={`${tugma("ikkilamchi", { ochiq: !etiborsizQila || g.status === "ignored" }).className} ${FOKUS}`}
              style={{
                ...tugma("ikkilamchi", { ochiq: !etiborsizQila || g.status === "ignored" }).style,
                height: 38,
                borderRadius: 9,
                paddingLeft: 16,
                paddingRight: 16,
              }}
            >
              <EyeOff size={15} aria-hidden />
              E'tiborsiz qoldirish
            </button>
            <button
              type="button"
              onClick={() => setOyna({ tur: "telegram" })}
              disabled={qoldi > 0}
              className={`${tugma("ikkilamchi", { ochiq: qoldi > 0 }).className} ${FOKUS}`}
              style={{
                ...tugma("ikkilamchi", { ochiq: qoldi > 0 }).style,
                height: 38,
                borderRadius: 9,
                paddingLeft: 16,
                paddingRight: 16,
              }}
            >
              <Send size={15} aria-hidden />
              {qoldi > 0 ? `Telegram — ${qoldi} s` : "Telegram'ga yuborish"}
            </button>
          </div>
        )}
      </div>

      {/* ══ Holatlar: ruxsat yo'q / xato / yuklanmoqda ═════════════ */}
      {ruxsatYoq ? (
        <div className="grid place-items-center rounded-[14px] bg-white px-6 py-[64px]" style={karta}>
          <Holat
            ikon={<ShieldOff size={34} color={XIRA_QUYUQ} aria-hidden />}
            sarlavha="Ruxsat yo'q"
            sarlavhaRang={IK}
            tavsif="Xatoliklar jurnali faqat superadmin va moderator uchun ochiq."
            amal={
              <Link
                href="/admin"
                className={`${tugma("ikkilamchi").className} ${FOKUS}`}
                style={{ ...tugma("ikkilamchi").style, height: 38 }}
              >
                Bosh sahifaga
              </Link>
            }
          />
        </div>
      ) : xato && !d ? (
        <div className="grid place-items-center rounded-[14px] bg-white px-6 py-[64px]" style={karta}>
          <Holat
            ikon={<CircleAlert size={34} color={QIZIL} aria-hidden />}
            sarlavha="Ma'lumotni yuklab bo'lmadi"
            sarlavhaRang={QIZIL}
            tavsif={xato.message || "Server javob bermadi. Ulanishni tekshirib, qayta urinib ko'ring."}
            amal={
              <button
                type="button"
                onClick={() => yukla()}
                className={`${tugma("asosiy").className} ${FOKUS}`}
                style={{ ...tugma("asosiy").style, height: 38 }}
              >
                Qayta urinish
              </button>
            }
          />
        </div>
      ) : !d || hozir === null ? (
        <Skelet />
      ) : (
        <>
          {/* ══ Xulosa paneli (Figma 396:2) ═══════════════════════ */}
          <div
            className="flex min-h-[104px] min-w-0 flex-wrap items-stretch rounded-[14px] bg-white"
            style={karta}
          >
            <XulosaUstun yorliq="Muhimlik">
              {dMeta ? (
                <Nishon nomi={dMeta.nomi} rang={dMeta.rang} matn={dMeta.matn} nuqta />
              ) : (
                <span className="text-[12.5px]" style={{ color: XIRA_QUYUQ }}>
                  {g!.severity}
                </span>
              )}
            </XulosaUstun>
            <Ajratgich />
            <XulosaUstun yorliq="Holat">
              {hMeta ? (
                <Nishon nomi={hMeta.nomi} rang={hMeta.rang} matn={hMeta.matn} nuqta />
              ) : (
                <span className="text-[12.5px]" style={{ color: XIRA_QUYUQ }}>
                  {g!.status}
                </span>
              )}
            </XulosaUstun>
            <Ajratgich />
            <XulosaUstun yorliq="Jami hodisalar">
              <span className="text-[28px] font-bold leading-[34px]" style={{ color: IK }}>
                {son(g!.count)}
              </span>
            </XulosaUstun>
            <Ajratgich />
            <XulosaUstun yorliq="Ta'sirlangan foydalanuvchi">
              <span className="text-[28px] font-bold leading-[34px]" style={{ color: KO_K }}>
                {son(g!.usersCount)}
              </span>
            </XulosaUstun>
            <Ajratgich />
            <XulosaUstun yorliq="Birinchi marta">
              <Vaqt iso={g!.firstSeenAt} hozir={hozir} />
            </XulosaUstun>
            <Ajratgich />
            <XulosaUstun yorliq="Oxirgi marta">
              <Vaqt iso={g!.lastSeenAt} hozir={hozir} />
            </XulosaUstun>
          </div>

          {/* ══ Ikki ustun (Figma 396:30 va 405:2) ════════════════ */}
          <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,1fr)_392px]">
            <div className="flex min-w-0 flex-col gap-3">
              <StackKarta d={d} hozir={hozir} nusxala={stackNusxa} />
              <ChiziqKarta d={d} />
              <HodisalarKarta d={d} hozir={hozir} />
              <TarixKarta
                g={d.group}
                hozir={hozir}
                // Izoh qoldirish moderatorga ham ochiq: bu holatni
                // o'zgartirmaydi, faqat tarixga yozuv qo'shadi. Support bu
                // yergacha kelmaydi — ekran yuqorida bloklangan.
                izohQosh={() => {
                  setMatn("");
                  setMatnXato(false);
                  setOyna({ tur: "izoh" });
                }}
              />
            </div>

            <div className="flex min-w-0 flex-col gap-3">
              {/* Kartaga endi BUTUN `d` kerak: bosqichlar tasmasi ostidagi
                  maydonlar `sinceStarted` va ta'sir taqsimotiga (qaysi
                  versiyalarda hali uchraydi) tayanadi. */}
              <HolatKarta
                d={d}
                hozir={hozir}
                ozgartirsaBoladi={ozgartirsaBoladi}
                holatBosildi={holatOch}
                masulBosildi={() => {
                  // Guruhda faqat mas'ulning YORLIG'I saqlanadi (`assigneeId`
                  // JSON'da ataylab yopiq), shuning uchun tanlovni nom
                  // bo'yicha topamiz (`masulId`).
                  setMasul(masulId(d.group.assignee));
                  setOyna({ tur: "masul" });
                }}
              />
              <MuhitKarta d={d} />
              <SorovKarta d={d} nusxala={sorovNusxa} />
              {/* Figma 3.12.3 · I ikkalasini BITTA bo'limga qo'ygan:
                  "kim duch keldi" va "qanday qadamlardan keyin". Shuning
                  uchun qadamlar aynan foydalanuvchi kartasidan keyin
                  turadi — admin odamni ko'rgach, darhol uning yo'lini
                  o'qiydi. Ustun ham to'g'ri keladi: ikkalasi ham
                  namunaning (`d.sample`) o'zidan chiqadi, chap ustundagi
                  kartalar esa BUTUN guruhni ko'rsatadi. */}
              <OdamlarKarta d={d} />
              <QadamlarKarta d={d} hozir={hozir} />
            </div>
          </div>

          {/* ══ Butun kenglikdagi uchlik ═══════════════════════════
              Tartib — savol ketma-ketligi bo'yicha: kimga tegdi
              (taqsimot) → nega bo'ldi (AI xulosasi) → nimaga tayangan
              (xom kontekst). Uchalasi ham ichida ko'p ustunli tuzilmaga
              ega va tor ustunda o'qib bo'lmasdi. */}
          <TasirKarta d={d} />
          <AiTahlil
            d={d}
            include={bolaklar}
            hozir={hozir}
            xabar={xabarQosh}
            guruhYangila={guruhYangila}
          />
          <AiKontekst
            d={d}
            xabar={xabarQosh}
            tanlangan={bolaklar}
            setTanlangan={setBolaklar}
            qoldi={qoldi}
            tgBoshla={setTgVaqt}
            guruhYangila={guruhYangila}
          />
        </>
      )}

      {/* ══ Oynalar ═══════════════════════════════════════════════ */}

      <AdminModal
        open={oyna?.tur === "holat"}
        onClose={oynaYop}
        title={
          oyna?.tur === "holat" && oyna.holat === "ignored"
            ? "E'tiborsiz qoldirilsinmi?"
            : "Holatni o'zgartirish"
        }
        maxWidth="max-w-[460px]"
        footer={
          <>
            <button
              type="button"
              onClick={oynaYop}
              disabled={saqlanmoqda}
              className={tugma("ikkilamchi", { ochiq: saqlanmoqda }).className}
              style={tugma("ikkilamchi", { ochiq: saqlanmoqda }).style}
            >
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={holatSaqla}
              disabled={saqlanmoqda}
              className={
                oyna?.tur === "holat" && oyna.holat === "ignored"
                  ? tugma("xavf", { ochiq: saqlanmoqda }).className
                  : tugma("asosiy", { ochiq: saqlanmoqda }).className
              }
              style={
                oyna?.tur === "holat" && oyna.holat === "ignored"
                  ? tugma("xavf", { ochiq: saqlanmoqda }).style
                  : tugma("asosiy", { ochiq: saqlanmoqda }).style
              }
            >
              {oyna?.tur === "holat" && oyna.holat === "ignored"
                ? tasdiq
                  ? "Ha, e'tiborsiz qoldirilsin"
                  : "Davom etish"
                : "Saqlash"}
            </button>
          </>
        }
      >
        {oyna?.tur === "holat" && g && (
          <div className="flex flex-col gap-3">
            <p className="text-[13px] leading-[19px]" style={{ color: KUL }}>
              <span className="font-semibold" style={{ color: IK }}>
                {g.ref}
              </span>{" "}
              guruhi{" "}
              <span className="font-semibold" style={{ color: HOLAT[oyna.holat].rang }}>
                «{HOLAT[oyna.holat].nomi}»
              </span>{" "}
              holatiga o'tkaziladi. {HOLAT[oyna.holat].izoh}
            </p>

            {/* Holatga BOG'LIQ maydonlar (Figma 3.12.3 · J · "Qo'shimcha
                maydonlar"). Ular izohdan YUQORIDA turadi: kartadagi tartib
                ham shunday ("Mas'ul dasturchi → Rejalashtirilgan versiya →
                Tuzatish izohi"), va admin avval faktni, keyin izohni
                yozgani qulay. */}
            {oyna.holat === "fixing" && (
              <label className="flex flex-col gap-[6px]">
                <span className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
                  Mas'ul dasturchi (ixtiyoriy)
                </span>
                <div className="relative">
                  <select
                    value={holatMasul}
                    onChange={(e) => setHolatMasul(e.target.value)}
                    className={`h-10 w-full appearance-none rounded-[10px] bg-white pl-[12px] pr-[30px] text-[13px] leading-[18px] outline-none ${FOKUS}`}
                    style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}`, color: IK }}
                  >
                    {/* Bo'sh qiymat = "tegmaslik". Server aynan shunday
                        tushunadi: mas'ul bor bo'lsa saqlanadi, bo'lmasa
                        amalni bajargan admin o'ziga oladi. */}
                    <option value="">
                      {g.assignee ? `O'zgarmasin · ${g.assignee}` : "Men olaman"}
                    </option>
                    {masullar.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.label} · {m.role}
                      </option>
                    ))}
                  </select>
                  <ChevronDown
                    size={15}
                    color={OCH_KUL}
                    aria-hidden
                    className="pointer-events-none absolute right-[12px] top-1/2 -translate-y-1/2"
                  />
                </div>
              </label>
            )}

            {(oyna.holat === "fixing" || oyna.holat === "resolved") && (
              <label className="flex flex-col gap-[6px]">
                <span className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
                  {oyna.holat === "fixing"
                    ? "Rejalashtirilgan versiya (ixtiyoriy)"
                    : "Tuzatilgan versiya (ixtiyoriy)"}
                </span>
                <input
                  type="text"
                  value={versiya}
                  onChange={(e) => setVersiya(e.target.value)}
                  maxLength={VERSIYA_ENG_KOP}
                  spellCheck={false}
                  placeholder={oyna.holat === "fixing" ? "Masalan: 1.4.3" : "Masalan: 1.4.3 (121)"}
                  className={`h-10 w-full rounded-[10px] bg-white px-[12px] text-[13px] leading-[18px] outline-none ${FOKUS}`}
                  style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}`, color: IK }}
                />
                {/* Ipucha bo'sh qolmasin: "ixtiyoriy" degan yorliq odatda
                    "kerak emas" deb o'qiladi, holbuki yopilish versiyasi
                    REGRESSIYA tekshiruvining tayanchi — xatolik qaytganda
                    tizim aynan shu qiymat bilan solishtiradi. */}
                <span className="text-[12px] leading-4" style={{ color: XIRA_QUYUQ }}>
                  {oyna.holat === "fixing"
                    ? "Qaysi chiqishda tuzatish rejalashtirilgan. Bo'sh qoldirilsa o'zgarmaydi."
                    : "Tuzatish chiqqan build. Xatolik qaytsa, shu versiya bilan solishtiriladi."}
                </span>
              </label>
            )}

            <label className="flex flex-col gap-[6px]">
              <span className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
                {/* "Bartaraf etildi" da izoh serverda `fixNote` ga tushadi va
                    kartada "Tuzatish izohi" deb ko'rinadi — yorliq ham
                    o'shanga mos bo'lsin, aks holda admin bir xil maydonni
                    ikki xil nom bilan ko'rardi. */}
                {oyna.holat === "ignored"
                  ? "Sabab (majburiy)"
                  : oyna.holat === "resolved"
                    ? "Tuzatish izohi (ixtiyoriy)"
                    : "Izoh (ixtiyoriy)"}
              </span>
              <textarea
                value={matn}
                onChange={(e) => {
                  setMatn(e.target.value);
                  if (matnXato) setMatnXato(false);
                }}
                rows={3}
                maxLength={300}
                placeholder={
                  oyna.holat === "ignored"
                    ? `Masalan: uchinchi tomon SDK'sining ma'lum nuqsoni, bizga bog'liq emas (kamida ${SABAB_ENG_KAM} belgi)`
                    : "Nima qilinganini yozib qoldiring"
                }
                className={`w-full resize-none rounded-[10px] bg-white px-[12px] py-[9px] text-[13px] leading-[18px] outline-none ${FOKUS}`}
                style={{
                  boxShadow: `inset 0 0 0 ${matnXato ? 1.5 : 1}px ${matnXato ? QIZIL_HOSHIYA : HOSHIYA_QUYUQ}`,
                  color: IK,
                }}
              />
              {matnXato && (
                <span className="text-[12px] leading-4" style={{ color: QIZIL }}>
                  Sabab kamida {SABAB_ENG_KAM} belgi bo'lishi kerak — keyin bu qarorni hech kim
                  tushuntira olmaydi.
                </span>
              )}
            </label>

            {oyna.holat === "ignored" && (
              <Izohcha kor="sariq" ikon={<TriangleAlert size={14} aria-hidden />}>
                E'tiborsiz qoldirilgan guruh hisobotlardan va ogohlantirishlardan chiqariladi —
                xatolik davom etsa ham hech kim xabar olmaydi. Amal audit jurnaliga yoziladi.
              </Izohcha>
            )}
            {tasdiq && (
              <Izohcha kor="sariq" ikon={<TriangleAlert size={14} aria-hidden />}>
                Tasdiqlang: <b>{g.ref}</b> guruhi e'tiborsiz qoldiriladi.
              </Izohcha>
            )}
          </div>
        )}
      </AdminModal>

      <AdminModal
        open={oyna?.tur === "izoh"}
        onClose={oynaYop}
        title="Izoh qo'shish"
        maxWidth="max-w-[460px]"
        footer={
          <>
            <button
              type="button"
              onClick={oynaYop}
              className={tugma("ikkilamchi").className}
              style={tugma("ikkilamchi").style}
            >
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={izohSaqla}
              disabled={saqlanmoqda}
              className={tugma("asosiy", { ochiq: saqlanmoqda }).className}
              style={tugma("asosiy", { ochiq: saqlanmoqda }).style}
            >
              Saqlash
            </button>
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <textarea
            value={matn}
            onChange={(e) => {
              setMatn(e.target.value);
              if (matnXato) setMatnXato(false);
            }}
            rows={4}
            maxLength={300}
            placeholder="Masalan: ApiClient'da javob null bo'lganda tekshiruv yo'q — 292-qatorda"
            className={`w-full resize-none rounded-[10px] bg-white px-[12px] py-[9px] text-[13px] leading-[18px] outline-none ${FOKUS}`}
            style={{
              boxShadow: `inset 0 0 0 ${matnXato ? 1.5 : 1}px ${matnXato ? QIZIL_HOSHIYA : HOSHIYA_QUYUQ}`,
              color: IK,
            }}
          />
          {matnXato && (
            <span className="text-[12px] leading-4" style={{ color: QIZIL }}>
              Izoh juda qisqa — kamida uchta belgi yozing.
            </span>
          )}
          <Izohcha>
            Izoh amallar tarixida sizning nomingiz bilan qoladi va uni o'chirib bo'lmaydi.
          </Izohcha>
        </div>
      </AdminModal>

      <AdminModal
        open={oyna?.tur === "masul"}
        onClose={oynaYop}
        title="Mas'ul adminni tanlash"
        maxWidth="max-w-[435px]"
        footer={
          <>
            <button
              type="button"
              onClick={oynaYop}
              className={tugma("ikkilamchi").className}
              style={tugma("ikkilamchi").style}
            >
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={masulSaqla}
              disabled={saqlanmoqda}
              className={tugma("asosiy", { ochiq: saqlanmoqda }).className}
              style={tugma("asosiy", { ochiq: saqlanmoqda }).style}
            >
              Biriktirish
            </button>
          </>
        }
      >
        <div className="flex flex-col">
          {[{ id: "", label: "Biriktirilmasin", role: "" }, ...masullar].map((m) => (
            <label
              key={m.id || "yoq"}
              className="flex min-h-[40px] cursor-pointer items-center gap-[10px]"
              style={{ boxShadow: `inset 0 -1px 0 0 ${HOSHIYA}` }}
            >
              <input
                type="radio"
                name="masul"
                className="peer sr-only"
                checked={masul === m.id}
                onChange={() => setMasul(m.id)}
              />
              <span
                aria-hidden
                className="grid h-[16px] w-[16px] shrink-0 place-items-center rounded-full peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-1 peer-focus-visible:outline-[#004ac6]"
                style={{
                  boxShadow: `inset 0 0 0 ${masul === m.id ? 5 : 1}px ${masul === m.id ? KO_K : HOSHIYA_QUYUQ}`,
                }}
              />
              <span className="min-w-0 text-[13px] leading-[18px]" style={{ color: m.id ? IK : OCH_KUL }}>
                {m.label}
                {m.role && (
                  <span className="ml-[6px] text-[12px]" style={{ color: OCH_KUL }}>
                    · {m.role}
                  </span>
                )}
              </span>
            </label>
          ))}
          <p className="pt-[10px] text-[12px] leading-4" style={{ color: XIRA_QUYUQ }}>
            Ro'yxatda faqat FAOL adminlar bor — o'chirilgan hisob mas'ul bo'lib qolmasligi kerak.
            O'zgarish audit jurnaliga va guruh tarixiga yoziladi.
          </p>
        </div>
      </AdminModal>

      <AdminModal
        open={oyna?.tur === "telegram"}
        onClose={oynaYop}
        title="Telegram'ga yuborilsinmi?"
        maxWidth="max-w-[460px]"
        footer={
          <>
            <button
              type="button"
              onClick={oynaYop}
              className={tugma("ikkilamchi").className}
              style={tugma("ikkilamchi").style}
            >
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={tgYubor}
              disabled={saqlanmoqda}
              className={tugma("asosiy", { ochiq: saqlanmoqda }).className}
              style={tugma("asosiy", { ochiq: saqlanmoqda }).style}
            >
              <Send size={15} aria-hidden />
              Yuborish
            </button>
          </>
        }
      >
        {g && (
          <div className="flex flex-col gap-3">
            <p className="text-[13px] leading-[19px]" style={{ color: KUL }}>
              <span className="font-semibold" style={{ color: IK }}>
                {g.ref}
              </span>{" "}
              — {g.title || g.code} · {son(g.count)} ta hodisa · ogohlantirish kanaliga
              (<span className="font-semibold">ERROR_ALERT_CHAT_ID</span>) yuboriladi.
            </p>
            <Izohcha kor="sariq">
              Bu ma'lumot paneldan TASHQARIGA chiqadi va kanaldagi hamma uni ko'radi. Telefon va IP
              niqoblangan, lekin xato matni, endpoint nomi va modul tuzilishi xabarda qoladi.
              Ketma-ket yuborishning oldini olish uchun 60 soniyalik oraliq qo'yiladi.
            </Izohcha>
          </div>
        )}
      </AdminModal>

      <Xabarlar xabarlar={xabarlar} yop={xabarYop} />
    </div>
  );
}

/* ── Kichik ko'rinish bo'laklari ───────────────────────────────────── */

function XulosaUstun({ yorliq, children }: { yorliq: string; children: React.ReactNode }) {
  return (
    <div className="flex min-w-[150px] flex-1 flex-col justify-center gap-[7px] px-[18px] py-[16px]">
      <p className="text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
        {yorliq}
      </p>
      {children}
    </div>
  );
}

function Ajratgich() {
  return <div aria-hidden className="w-px self-stretch" style={{ background: HOSHIYA }} />;
}

/** Xulosa panelidagi sana: "12.08.2026 · 09:14" yoki "Bugun · 14:21". */
function Vaqt({ iso, hozir }: { iso?: string; hozir: number }) {
  const d = iso ? new Date(iso) : null;
  const yaroqli = d && !Number.isNaN(d.getTime());
  return (
    <span
      className="text-[13.5px] font-semibold leading-[18px]"
      style={{ color: yaroqli ? IK : XIRA_QUYUQ }}
      title={yaroqli ? kunSoat(d!, hozir) : undefined}
    >
      {yaroqli ? kunNuqta(d!, hozir) : YOQ}
    </span>
  );
}

/** Bo'sh/xato holat karkasi — ro'yxat sahifasidagi bilan bir xil. */
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
  amal?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-[10px] text-center">
      {ikon}
      <div className="text-[16px] font-semibold leading-[22px]" style={{ color: sarlavhaRang }}>
        {sarlavha}
      </div>
      <p className="max-w-[460px] text-[13px] leading-[18px]" style={{ color: OCH_KUL }}>
        {tavsif}
      </p>
      {amal && <div className="mt-[4px]">{amal}</div>}
    </div>
  );
}

/**
 * Yuklanish skeleti.
 *
 * Bloklarning o'lchami tayyor ekrandagiga yaqin: sahifa yuklangach
 * kartalar sakrab ketmasin.
 */
function Skelet() {
  const bar = (w: string, h = 14) => (
    <div className="rounded-[6px]" style={{ background: HOSHIYA_OCH, width: w, height: h }} />
  );
  const qobiq: React.CSSProperties = { boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${SOYA}` };

  return (
    <div className="flex min-w-0 animate-pulse flex-col gap-4" aria-hidden>
      <div className="flex min-h-[104px] items-center gap-4 rounded-[14px] bg-white px-[18px]" style={qobiq}>
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <div key={i} className="flex flex-1 flex-col gap-[10px]">
            {bar("70%", 12)}
            {bar("50%", 20)}
          </div>
        ))}
      </div>
      <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,1fr)_392px]">
        <div className="flex min-w-0 flex-col gap-3">
          {[220, 220, 320].map((h, i) => (
            <div key={i} className="rounded-[14px] bg-white" style={{ ...qobiq, height: h }} />
          ))}
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          {/* Beshta qiymat — o'ng ustundagi beshta kartaning (holat, muhit,
              so'rov, foydalanuvchilar, qadamlar) taxminiy balandligi. */}
          {[170, 260, 300, 240, 260].map((h, i) => (
            <div key={i} className="rounded-[14px] bg-white" style={{ ...qobiq, height: h }} />
          ))}
        </div>
      </div>
    </div>
  );
}
