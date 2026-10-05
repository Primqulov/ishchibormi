"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { Download } from "lucide-react";
import { api, Elon, Category, Paged, downloadAdminCsv, getAdminRole, APIError } from "@/lib/api";
import { DeleteModeModal, DeleteMode } from "@/components/admin/DeleteModeModal";
import { Pagination } from "@/components/Pagination";
import { MatnFiltr, Tanlov } from "@/components/admin/Filtr";
import { MuddatKaliti } from "@/components/admin/MuddatKaliti";
import { ELON_HOLAT, ELON_HOLATLARI, ElonNishoni, elonHolatKor } from "@/components/admin/ElonHolat";
import {
  HOSHIYA,
  IK,
  KUL,
  OCH_KUL,
  QIZIL,
  QIZIL_FON,
  SARLAVHA_FON,
  SOYA,
  XIRA_QUYUQ,
  tugma,
} from "@/components/admin/ui";

/* ─────────────────────────────────────────────────────────────────────
   Figma: "3.5 · E'lonlar — ro'yxat (1440×1024)" va
          "3.5a · E'lonlar — holatlar, amallar va oynalar".

   O'lchamlar Figma'dan aynan olingan: sarlavha kartasi 70, filtr paneli
   66, jadval sarlavhasi 46, qator 52, sahifalash 56 px. Ustunlar:
   Sarlavha 311 · Turkum 173 · Holat 184 · Ishchilar 115 · Narx 161 ·
   Amallar 208 (jami 1152).

   Figma'dagi hoshiya INSIDE turadi — CSS `border` qutini kattartirib
   yuborardi, shuning uchun hamma joyda `inset` box-shadow ishlatilgan.
   ───────────────────────────────────────────────────────────────────── */

const LIMIT = 20;

/* Filtrlarning RUXSAT ETILGAN qiymatlari.
 *
 * Backend ham o'z tomonida tekshiradi (elonsFilter tanish bo'lmagan
 * holatni shunchaki topmaydi), lekin so'rov yuborilishidan OLDIN oq
 * ro'yxatdan o'tkazish bu yerdagi holat buzilib qolsa ham kutilmagan
 * parametr ketishining oldini oladi. */
const HOLAT_QIY: readonly string[] = ["", ...ELON_HOLATLARI];
const OCHIRILGAN_QIY: readonly string[] = ["", "hide", "only"];

const oq = (v: string, ruxsat: readonly string[]) => (ruxsat.includes(v) ? v : "");

/**
 * Turkum identifikatori — faqat 24 belgili hex (MongoDB ObjectID).
 *
 * Ro'yxat serverdan keladi, ya'ni qiymat "ishonchli" ko'rinadi. Lekin
 * `<select>` qiymati brauzer vositalari orqali o'zgartirilishi mumkin, va
 * bu satr to'g'ridan-to'g'ri so'rovga tushadi. Shakl tekshirilmasa,
 * serverga ma'nosiz parametr ketardi (u e'tiborsiz qoldiradi — lekin
 * so'rov baribir "filtrlangan" deb ko'rinardi va admin noto'g'ri
 * ro'yxatga qarab qaror qabul qilardi).
 */
const oqOid = (v: string) => (/^[0-9a-f]{24}$/i.test(v) ? v : "");

/** Ming ajratgichi — uzuq bo'shliq (Figma: "450 000"). */
function son(v: number): string {
  if (!Number.isFinite(v)) return "—";
  return Math.max(0, Math.round(v))
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

export default function AdminElons() {
  const [data, setData] = useState<Paged<Elon> | null>(null);
  const [cats, setCats] = useState<Category[]>([]);
  const [page, setPage] = useState(1);
  // Qidiruv maydonlari darhol emas, kechikib so'rovga aylanadi — aks holda
  // har bosilgan harf uchun alohida so'rov ketardi.
  const [qKirit, setQKirit] = useState("");
  const [viloyatKirit, setViloyatKirit] = useState("");
  const [q, setQ] = useState("");
  const [region, setRegion] = useState("");
  const [status, setStatus] = useState("");
  const [categoryId, setCategoryId] = useState("");
  // "O'chirilgan" filtri: bo'sh = hammasi (standart), chunki olib tashlangan
  // e'lon admin panelida ko'rinib turishi kerak.
  const [deleted, setDeleted] = useState("");
  const [yuklanmoqda, setYuklanmoqda] = useState(true);
  const [royxatXato, setRoyxatXato] = useState("");
  // Yashirish/Tiklash tasdiqsiz, bir bosishda ishlaydi (Figma 3.5a · 3).
  // Shuning uchun xatosi ham ko'rinib turishi SHART: aks holda admin
  // "bosdim, bo'ldi" deb o'ylab ketardi, e'lon esa ochiq qolardi.
  const [amalXato, setAmalXato] = useState("");
  // Amal ketayotgan e'lonlarning `id`lari — tugma ikki marta bosilmasin.
  //
  // Ro'yxat, bitta satr emas: ikkita boshqa e'lonni ketma-ket yashirish
  // mumkin bo'lishi kerak. Umumiy "band" bayrog'i ikkinchi tugmani ochiq
  // qoldirib, bosilganda esa jimgina e'tiborsiz qoldirardi.
  const [amalBand, setAmalBand] = useState<string[]>([]);
  // O'chirish oynasi butun `Elon` ni oladi, `id` ni emas: oyna QAYSI
  // e'lon o'chirilayotganini sarlavhasi bilan ko'rsatadi. Bu tasodifan
  // yonidagi qatorni bosishdan saqlaydigan eng arzon himoya.
  const [delTarget, setDelTarget] = useState<Elon | null>(null);
  const [delBusy, setDelBusy] = useState(false);
  const [delErr, setDelErr] = useState("");
  // Bazadan butunlay o'chirish faqat superadminga ko'rsatiladi. Server
  // tomonida ham alohida tekshiriladi — bu faqat interfeys.
  const [isSuper, setIsSuper] = useState(false);

  useEffect(() => {
    const t = setTimeout(() => setQ(qKirit.trim()), 300);
    return () => clearTimeout(t);
  }, [qKirit]);
  useEffect(() => {
    const t = setTimeout(() => setRegion(viloyatKirit.trim()), 300);
    return () => clearTimeout(t);
  }, [viloyatKirit]);

  // Har so'rovga tartib raqami beriladi: sekin qaytgan ESKI javob yangi
  // filtr natijasini bosib ketmasin.
  const soravRaqami = useRef(0);

  const load = useCallback(async () => {
    const men = ++soravRaqami.current;
    setYuklanmoqda(true);
    try {
      const params = new URLSearchParams({
        page: String(Math.max(1, Math.floor(page))),
        limit: String(LIMIT),
      });
      if (q) params.set("q", q);
      if (region) params.set("region", region);
      if (oq(status, HOLAT_QIY)) params.set("status", status);
      if (oqOid(categoryId)) params.set("categoryId", categoryId);
      if (oq(deleted, OCHIRILGAN_QIY)) params.set("deleted", deleted);
      const javob = await api.get<Paged<Elon>>(`/api/admin/elons?${params}`, { auth: "admin" } as any);
      if (men !== soravRaqami.current) return;
      setData(javob);
      setRoyxatXato("");
    } catch (e) {
      if (men !== soravRaqami.current) return;
      setData(null);
      setRoyxatXato((e as APIError)?.message || "Ro'yxatni yuklab bo'lmadi");
    } finally {
      if (men === soravRaqami.current) setYuklanmoqda(false);
    }
  }, [page, q, region, status, categoryId, deleted]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setIsSuper(getAdminRole() === "superadmin"); }, []);
  // Filtr o'zgarsa 1-sahifaga qaytamiz.
  useEffect(() => { setPage(1); }, [q, region, status, categoryId, deleted]);
  useEffect(() => {
    // Turkumlar ro'yxati filtr uchun. Kelmasa — filtr faqat "barchasi"
    // bilan qoladi, sahifa esa ishlashda davom etadi.
    api.get<Category[]>("/api/admin/categories", { auth: "admin" } as any)
      .then((r) => setCats(Array.isArray(r) ? r : []))
      .catch(() => {});
  }, []);

  /**
   * Yashirish / Tiklash — Figma 3.5a · 3: yagona QAYTARILADIGAN amal,
   * shuning uchun tasdiq oynasi yo'q.
   *
   * «Tiklash» serverga "recruiting" yuboradi, lekin server e'lonni
   * yashirishdan OLDINGI holatiga qaytaradi (hiddenFromStatus). Ya'ni
   * boshlanib ketgan ish qaytadan ariza qabul qila boshlamaydi.
   */
  async function holatniOzgartir(e: Elon, yangi: "hidden" | "recruiting") {
    if (amalBand.includes(e.id)) return;
    if (yangi === "hidden" && !window.confirm("E'lon yashiriladi va rasmlari butunlay o'chiriladi. Tiklash rasmlarni qaytarmaydi. Davom etilsinmi?")) return;
    setAmalBand((v) => [...v, e.id]);
    setAmalXato("");
    try {
      await api.patch(
        `/api/admin/elons/${encodeURIComponent(e.id)}/status`,
        { status: yangi, expectedStatus: e.status, expectedUpdatedAt: e.updatedAt },
        { auth: "admin" } as any,
      );
      await load();
    } catch (err) {
      setAmalXato(
        (err as APIError)?.message ||
          (yangi === "hidden" ? "E'lonni yashirib bo'lmadi" : "E'lonni tiklab bo'lmadi"),
      );
    } finally {
      setAmalBand((v) => v.filter((x) => x !== e.id));
    }
  }

  async function del(mode: DeleteMode) {
    if (!delTarget) return;
    setDelBusy(true);
    setDelErr("");
    try {
      await api.delete(
        `/api/admin/elons/${encodeURIComponent(delTarget.id)}?mode=${encodeURIComponent(mode)}`,
        { auth: "admin" } as any,
      );
      setDelTarget(null);
      await load();
    } catch (e) {
      setDelErr((e as APIError)?.message || "O'chirib bo'lmadi");
    } finally {
      setDelBusy(false);
    }
  }

  function exportCsv() {
    // Figma 3.5a · 6: "Tugma ekrandagi filtrlarni saqlab qoladi."
    //
    // Bu ekranda `deleted` HAM yuboriladi — foydalanuvchilar ekranidan
    // farqli. Sabab: bu ro'yxatning standart holati "o'chirilganlar bilan",
    // ya'ni admin ko'rib turgan jadval o'chirilganlarni ham o'z ichiga
    // oladi. Filtrni tashlab ketsak, yuklab olingan fayl ekrandagidan
    // boshqa bo'lib chiqardi — eksport esa aynan "ko'rib turganimni
    // saqlab qo'yish" uchun bosiladi.
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    if (region) params.set("region", region);
    if (oq(status, HOLAT_QIY)) params.set("status", status);
    if (oqOid(categoryId)) params.set("categoryId", categoryId);
    if (oq(deleted, OCHIRILGAN_QIY)) params.set("deleted", deleted);
    downloadAdminCsv("/api/admin/export/elons.csv", params);
  }

  const total = useMemo(() => {
    const t = data?.total;
    return typeof t === "number" && Number.isFinite(t) ? Math.max(0, Math.floor(t)) : 0;
  }, [data]);
  const pages = Math.max(1, Math.ceil(total / LIMIT));
  const elons = data?.items ?? [];
  const csvUslub = tugma("ikkilamchi");
  const karta: React.CSSProperties = { boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${SOYA}` };
  const ustun = (px: number) => ({ width: `${(px / 1152) * 100}%` });

  return (
    <div className="flex flex-col gap-4">
      {/* ── Sarlavha kartasi (Figma 60:180 — 70 px) ──────────────────── */}
      <div className="flex h-[70px] items-center justify-between gap-3 rounded-[14px] bg-white px-5" style={karta}>
        <div className="min-w-0">
          <h1 className="text-[18px] font-bold leading-6" style={{ color: IK }}>E&apos;lonlar</h1>
          <p className="mt-[2px] text-[12px] leading-4" style={{ color: OCH_KUL }}>
            Jami {son(total)} ta e&apos;lon
          </p>
        </div>
        <button onClick={exportCsv} className={`${csvUslub.className} w-[155px] shrink-0`} style={csvUslub.style}>
          <Download size={15} aria-hidden />
          CSV yuklab olish
        </button>
      </div>

      <MuddatKaliti ozgartiraOladi={isSuper} />

      {/* ── Filtr + jadval kartasi ───────────────────────────────────── */}
      <div className="overflow-hidden rounded-[14px] bg-white" style={karta}>
        {/* Figma 60:191: balandlik 66 (py 14 + maydon 38), oraliq 8. */}
        <div className="flex flex-wrap items-center gap-[8px] px-5 py-[14px]">
          <MatnFiltr
            nomi="Sarlavha bo'yicha qidirish"
            kenglik={250}
            qidiruv
            placeholder="Sarlavha bo'yicha qidirish…"
            qiymat={qKirit}
            maxLength={80}
            ozgardi={setQKirit}
          />
          <Tanlov nomi="Holat" kenglik={170} qiymat={status} ozgardi={(v) => setStatus(oq(v, HOLAT_QIY))}>
            <option value="">Holat (barchasi)</option>
            {ELON_HOLATLARI.map((s) => (
              <option key={s} value={s}>{ELON_HOLAT[s].matn}</option>
            ))}
          </Tanlov>
          <Tanlov nomi="Turkum" kenglik={180} qiymat={categoryId} ozgardi={(v) => setCategoryId(oqOid(v))}>
            <option value="">Turkum (barchasi)</option>
            {cats.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </Tanlov>
          <MatnFiltr
            nomi="Viloyat bo'yicha filtr"
            kenglik={140}
            placeholder="Viloyat"
            qiymat={viloyatKirit}
            maxLength={60}
            ozgardi={setViloyatKirit}
          />
          <Tanlov nomi="O'chirilganlar" kenglik={190} qiymat={deleted} ozgardi={(v) => setDeleted(oq(v, OCHIRILGAN_QIY))}>
            <option value="">O&apos;chirilganlar bilan</option>
            <option value="hide">Faqat faollari</option>
            <option value="only">Faqat o&apos;chirilganlari</option>
          </Tanlov>
          <div className="ml-auto text-[12px] font-medium leading-4" style={{ color: OCH_KUL }}>
            Jami: {son(total)}
          </div>
        </div>
        <div className="h-px" style={{ background: HOSHIYA }} />

        {/* Yashirish/Tiklash xatosi. Figma'da bu yo'lak yo'q, chunki
            Figma xato holatini chizmagan — lekin amal tasdiqsiz bajarilgani
            uchun jimgina yo'qolgan xato eng yomon variant bo'lardi. */}
        {amalXato && (
          <>
            <div
              role="status"
              aria-live="polite"
              className="px-5 py-[10px] text-[12px] font-medium leading-[17px]"
              style={{ background: QIZIL_FON, color: QIZIL }}
            >
              {amalXato}
            </div>
            <div className="h-px" style={{ background: HOSHIYA }} />
          </>
        )}

        <div className="overflow-x-auto">
          <table className="w-full min-w-[1000px] table-fixed border-collapse text-[14px]">
            {/* Foizga aylantirilgan — panel kengligi ekranga qarab
                o'zgaradi, ustunlar nisbati esa qolishi kerak. */}
            <colgroup>
              <col style={ustun(311)} />
              <col style={ustun(173)} />
              <col style={ustun(184)} />
              <col style={ustun(115)} />
              <col style={ustun(161)} />
              <col style={ustun(208)} />
            </colgroup>
            <thead>
              {/* Figma: sarlavha yo'lagi 46 px, matn Semi Bold 12/16. */}
              <tr style={{ background: SARLAVHA_FON }}>
                <th className="h-[46px] pl-5 pr-3 text-left text-[12px] font-semibold leading-4" style={{ color: IK }}>Sarlavha</th>
                <th className="px-3 text-left text-[12px] font-semibold leading-4" style={{ color: IK }}>Turkum</th>
                <th className="px-3 text-left text-[12px] font-semibold leading-4" style={{ color: IK }}>Holat</th>
                <th className="px-3 text-left text-[12px] font-semibold leading-4" style={{ color: IK }}>Ishchilar</th>
                <th className="px-3 text-left text-[12px] font-semibold leading-4" style={{ color: IK }}>Narx</th>
                <th className="pl-3 pr-5 text-right text-[12px] font-semibold leading-4" style={{ color: IK }}>Amallar</th>
              </tr>
            </thead>
            <tbody>
              {elons.map((e) => {
                const band = amalBand.includes(e.id);
                const neg = e.pricingType === "negotiable";
                return (
                  <tr key={e.id} className="h-[52px] border-b last:border-b-0" style={{ borderColor: HOSHIYA }}>
                    {/* O'chirilgan e'londa sarlavha xira bo'ladi va yonida
                        qizil "o'chirilgan" nishoni turadi. Holat ustuni
                        bilan ziddiyat yo'q: u yerda e'lonning ish holati
                        ("bekor qilingan"), bu yerda esa yozuvning taqdiri. */}
                    <td className="pl-5 pr-3 align-middle">
                      <div className="flex min-w-0 items-center gap-[8px]">
                        {/* Sarlavha — batafsil sahifaga havola. Amallar
                            ustunidagi tugmalar e'londan CHIQMAY qaror
                            qabul qilish uchun, bu havola esa qaror
                            qabul qilishdan OLDIN e'lonni to'liq o'qish
                            uchun. */}
                        <Link
                          href={`/admin/elons/${encodeURIComponent(e.id)}`}
                          className="truncate text-[13px] font-medium leading-[18px] hover:underline"
                          style={{ color: e.isDeleted ? OCH_KUL : IK }}
                          title={e.title}
                        >
                          {e.title || "—"}
                        </Link>
                        {e.isDeleted && <ElonNishoni matn="o'chirilgan" rang={QIZIL} />}
                      </div>
                    </td>
                    <td className="truncate px-3 align-middle text-[12px] leading-[17px]" style={{ color: KUL }}>
                      {e.categoryName || "—"}
                    </td>
                    <td className="px-3 align-middle">
                      <ElonNishoni {...elonHolatKor(e.status)} />
                    </td>
                    {/* Bitta son — kerakli ishchi soni. Qabul qilinganlar
                        soni ATAYLAB yo'q: ustun sarlavhasi ("Ishchilar")
                        bitta qiymat va'da qiladi, "2/5" ko'rinishi esa
                        Figma'da chizilmagan. */}
                    <td className="px-3 align-middle text-[13px] font-medium leading-[18px]" style={{ color: IK }}>
                      {Number.isFinite(e.workersNeeded) ? e.workersNeeded : "—"}
                    </td>
                    <td className="px-3 align-middle">
                      <div className="flex items-baseline gap-[4px] whitespace-nowrap">
                        <span className="text-[13px] font-semibold leading-[18px]" style={{ color: IK }}>
                          {neg ? "kelishiladi" : son(e.priceAmount)}
                        </span>
                        {!neg && (
                          <span className="text-[11px] leading-4" style={{ color: XIRA_QUYUQ }}>so&apos;m</span>
                        )}
                      </div>
                    </td>
                    <td className="pl-3 pr-5 align-middle">
                      <div className="flex items-center justify-end gap-[6px]">
                        {/* O'chirilgan e'londa «Yashirish/Tiklash» umuman
                            chizilmaydi (Figma 3.5a · 2) — u allaqachon
                            ko'rinmaydi va qaytmaydi. Server ham shu qoidani
                            qo'llaydi: 409 `elon_deleted`. */}
                        {!e.isDeleted && (
                          <button
                            onClick={() => holatniOzgartir(e, e.status === "hidden" ? "recruiting" : "hidden")}
                            disabled={band}
                            {...tugma("ikkilamchi", { kichik: true, ochiq: band })}
                          >
                            {e.status === "hidden" ? "Tiklash" : "Yashirish"}
                          </button>
                        )}
                        {(!e.isDeleted || isSuper) && (
                          <button
                            onClick={() => { setDelErr(""); setDelTarget(e); }}
                            {...tugma("xavf", { kichik: true })}
                          >
                            {e.isDeleted ? "Bazadan o'chirish" : "O'chirish"}
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
              {/* Bo'sh holat — Figma 3.5a · 5: jadval o'rnida 120 px
                  balandlikda bitta yozuv. Xato ham shu yerda chiqadi:
                  ro'yxat kelmaganda admin bo'sh jadvalni "e'lon yo'q" deb
                  o'qimasligi kerak. */}
              {!elons.length && (
                <tr>
                  <td
                    colSpan={6}
                    className="h-[120px] px-5 text-center text-[13px] font-medium leading-[18px]"
                    style={{ color: royxatXato ? QIZIL : XIRA_QUYUQ }}
                  >
                    {yuklanmoqda ? "Yuklanmoqda…" : royxatXato || "Hech narsa topilmadi"}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {pages > 1 && (
          <>
            <div className="h-px" style={{ background: HOSHIYA }} />
            <div className="px-5 py-[14px]">
              <Pagination page={page} pages={pages} onPage={setPage} />
            </div>
          </>
        )}
      </div>

      {/* Figma 3.5a · 4: foydalanuvchilar ekranidagi bilan BITTA komponent —
          faqat matndagi "hisob" so'zi "e'lon" ga almashadi.

          `tasdiq` ataylab berilmagan: bu ekranda yozib tasdiqlash faqat
          "bazadan butunlay o'chirish" uchun talab qilinadi (komponentning
          standart xulqi), "foydalanuvchilardan olib tashlash" esa ikki
          bosishda bajariladi — xuddi Figma'da chizilgani kabi. */}
      <DeleteModeModal
        open={!!delTarget}
        title="E'lonni o'chirish"
        what="e'lon"
        canPurge={isSuper}
        kim={delTarget ? {
          yorliq: "E'lon",
          nomi: delTarget.title || "(sarlavhasiz)",
          tafsilot: [delTarget.categoryName, delTarget.region, elonHolatKor(delTarget.status).matn]
            .filter(Boolean)
            .join(" · "),
        } : undefined}
        busy={delBusy}
        error={delErr}
        onCancel={() => setDelTarget(null)}
        onConfirm={del}
      />
    </div>
  );
}
