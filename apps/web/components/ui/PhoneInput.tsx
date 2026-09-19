"use client";
import { Phone } from "lucide-react";
import { fmtPhoneLocal } from "@/lib/format";

/**
 * O'zbekiston telefon raqami maydoni: "+998" QOTIRILGAN prefiks, foydalanuvchi
 * faqat 9 xonali milliy qismni yozadi.
 *
 * NEGA prefiks input MATNI emas: ilgari maydonda to'liq "+998 90 ..." satri
 * turardi va kursor "+998" ning ichiga kirib qolar, uni o'chirishga urinish
 * mumkin edi. Bu yerda "+998" — alohida, tanlanmaydigan element: uni o'chirib
 * ham, ustiga yozib ham bo'lmaydi. Flutter ilovasidagi `_PhoneField` aynan
 * shunday ishlaydi (post_job_page.dart), ya'ni ikkala platformada bir xil.
 *
 * [value] va [onChange] faqat MILLIY qism bilan ishlaydi ("90 123 45 67").
 * Serverga yuborishdan oldin `fmtPhone` bilan to'liq ko'rinishga o'giriladi —
 * ya'ni saqlanadigan format o'zgarmaydi.
 */
export function PhoneInput({
  value,
  onChange,
  required = false,
  className = "",
}: {
  /** Milliy qism, guruhlangan yoki xom — ikkalasi ham bo'ladi. */
  value: string;
  /** Guruhlangan milliy qism qaytadi: "90 123 45 67". */
  onChange: (localPart: string) => void;
  required?: boolean;
  className?: string;
}) {
  return (
    // Ramka .input dagi kabi, lekin tashqi o'ramda: fokus halqasi butun
    // maydonni (prefiks bilan birga) o'rab olsin.
    <div
      className={
        "flex items-center gap-2 rounded-lg border px-3 " +
        "border-[color:var(--border-strong)] bg-[color:var(--bg-subtle)] " +
        "transition focus-within:border-[color:var(--brand)] " +
        "focus-within:bg-[color:var(--card)] " +
        "focus-within:shadow-[0_0_0_3px_var(--ring)] " +
        className
      }
    >
      <Phone size={15} className="shrink-0" style={{ color: "var(--brand)" }} />
      {/* aria-hidden: skrinrider uchun raqamni input'ning o'zi aytadi
          (aria-label), prefiks ikki marta o'qilmasin. */}
      <span
        aria-hidden="true"
        className="select-none text-sm font-bold heading"
      >
        +998
      </span>
      <input
        className="min-w-0 flex-1 bg-transparent py-3 text-sm outline-none focus-visible:shadow-none"
        style={{ color: "var(--text)" }}
        required={required}
        inputMode="numeric"
        autoComplete="tel-national"
        aria-label="Telefon raqami, +998 dan keyingi qismi"
        value={fmtPhoneLocal(value)}
        onChange={(e) => onChange(fmtPhoneLocal(e.target.value))}
        placeholder="90 123 45 67"
      />
    </div>
  );
}
