"use client";
import { useCallback, useEffect, useState } from "react";
import { api, APIError, FeedSettings } from "@/lib/api";
import { HOSHIYA, HOSHIYA_QUYUQ, IK, KO_K, KUL, OCH_KUL, QIZIL, SARLAVHA_FON, SOYA, XIRA } from "./ui";

/* ─────────────────────────────────────────────────────────────────────
   «Muddati o'tgan ishlar» kaliti — E'lonlar sahifasining boshida.

   Bu ro'yxat FILTRI emas: pastdagi jadval hamma e'lonni har doim
   ko'rsatadi. Kalit FOYDALANUVCHILAR ko'radigan ro'yxatlarni (sayt, ilova,
   xarita) boshqaradi — shuning uchun filtr panelidan alohida kartada
   turadi va matni «foydalanuvchilarga» so'zi bilan yozilgan.

   Figma'da bu karta chizilmagan; o'lcham va ranglar yonidagi sarlavha
   kartasidan (3.5 · 60:180) olingan.
   ───────────────────────────────────────────────────────────────────── */

const son = (v: number) =>
  Math.max(0, Math.round(Number.isFinite(v) ? v : 0))
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, " ");

function izoh(s: FeedSettings): string {
  const n = son(s.expiredCount);
  if (s.showExpired) {
    return s.expiredCount > 0
      ? `Barcha ochiq ishlar ko'rinadi — shundan ${n} tasining sanasi o'tgan.`
      : "Barcha ochiq ishlar ko'rinadi. Hozir sanasi o'tgan ochiq ish yo'q.";
  }
  return s.expiredCount > 0
    ? `Sanasi o'tgan ${n} ta ish foydalanuvchilardan yashirilgan.`
    : "Sanasi o'tgan ishlar yashiriladi. Hozir bunday ochiq ish yo'q.";
}

export function MuddatKaliti({ ozgartiraOladi, ozgardi }: {
  /** Faqat superadmin almashtira oladi; qolganlar holatni ko'radi. */
  ozgartiraOladi: boolean;
  /** Kalit almashgach chaqiriladi — sahifa o'z ma'lumotini yangilashi uchun. */
  ozgardi?: () => void;
}) {
  const [holat, setHolat] = useState<FeedSettings | null>(null);
  const [band, setBand] = useState(false);
  const [xato, setXato] = useState("");

  const yukla = useCallback(async () => {
    try {
      setHolat(await api.get<FeedSettings>("/api/admin/settings/feed", { auth: "admin" } as any));
      setXato("");
    } catch (e) {
      setXato((e as APIError)?.message || "Sozlamani yuklab bo'lmadi");
    }
  }, []);
  useEffect(() => { yukla(); }, [yukla]);

  async function tanla(showExpired: boolean) {
    if (band || !holat || holat.showExpired === showExpired) return;
    setBand(true);
    setXato("");
    try {
      setHolat(await api.put<FeedSettings>("/api/admin/settings/feed", { showExpired }, { auth: "admin" } as any));
      ozgardi?.();
    } catch (e) {
      // Holat o'zgartirilmaydi: tugma serverdagi haqiqiy qiymatda qoladi.
      setXato((e as APIError)?.message || "Sozlamani saqlab bo'lmadi");
    } finally {
      setBand(false);
    }
  }

  const ochiq = !ozgartiraOladi || band || !holat;
  const bolak = (showExpired: boolean, matn: string) => {
    const tanlangan = holat?.showExpired === showExpired;
    return (
      <button
        type="button"
        role="radio"
        aria-checked={tanlangan}
        disabled={ochiq}
        onClick={() => tanla(showExpired)}
        className={`h-7 whitespace-nowrap rounded-[7px] px-3 text-[12px] font-semibold transition-colors ${ochiq ? "cursor-not-allowed" : ""}`}
        style={
          tanlangan
            ? { background: "#fff", color: KO_K, boxShadow: `inset 0 0 0 1px ${HOSHIYA_QUYUQ}` }
            : { color: ochiq ? XIRA : KUL }
        }
      >
        {matn}
      </button>
    );
  };

  return (
    <div
      className="flex min-h-[70px] flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-[14px] bg-white px-5 py-[14px]"
      style={{ boxShadow: `inset 0 0 0 1px ${HOSHIYA}, ${SOYA}` }}
    >
      <div className="min-w-0">
        <h2 className="text-[14px] font-semibold leading-5" style={{ color: IK }}>
          Muddati o&apos;tgan ishlar
        </h2>
        <p
          role="status"
          aria-live="polite"
          className="mt-[2px] text-[12px] leading-4"
          style={{ color: xato ? QIZIL : OCH_KUL }}
        >
          {xato || (holat ? izoh(holat) : "Yuklanmoqda…")}
          {!xato && holat && !ozgartiraOladi && " O'zgartirish faqat superadminga ochiq."}
        </p>
      </div>
      <div
        role="radiogroup"
        aria-label="Muddati o'tgan ishlar foydalanuvchilarga"
        className="flex shrink-0 gap-[2px] rounded-[10px] p-[3px]"
        style={{ background: SARLAVHA_FON }}
      >
        {bolak(true, "Barchasini ko'rsatish")}
        {bolak(false, "Muddati o'tganini yashirish")}
      </div>
    </div>
  );
}
