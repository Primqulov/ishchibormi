"use client";
import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation } from "@tanstack/react-query";
import { api, APIError, Category, Elon, Gender, GENDER_LABEL, GENDER_OPTIONS } from "@/lib/api";
import { Shell } from "@/components/Shell";
import { ModerationModal, isModerationError } from "@/components/ModerationModal";
import { CategoryIcon } from "@/components/CategoryIcon";
import { MultiImageUploader } from "@/components/ui/ImageUpload";
import { MapPicker, LatLng } from "@/components/ui/MapPicker";
import { CheckCircle2, AlertTriangle, User2 } from "lucide-react";
import { T, useT } from "@/components/T";
import { fmtSum, fmtThousands, onlyDigits, fmtPhone, phoneDigits } from "@/lib/format";
import { PhoneInput } from "@/components/ui/PhoneInput";

type Form = {
  title: string;
  categoryId: string;
  description: string;
  loc: LatLng | null;
  workersNeeded: number | "";
  gender: Gender;
  pricingType: "per_worker" | "total";
  priceAmount: number | "";
  startDate: string;
  workTimeFrom: string;
  contactPhone: string;
  images: string[];
};

function pad(n: number) {
  return String(n).padStart(2, "0");
}
function ymd(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
function hm(d: Date) {
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function CreateElonForm({ embedded = false, onCreated, onCancel }: { embedded?: boolean; onCreated?: (job: Elon) => void; onCancel?: () => void }) {
  const router = useRouter();
  const t = useT();

  // Eng erta joylashtirish vaqti — hozirgi vaqtdan 1 soat keyin (sahifa
  // ochilgan payt asos). Sana esa faqat 3 kun ichida: bugun, erta yoki indin.
  const startRef = useRef(new Date());
  const creationKey = useRef<string>("");
  // Soniya/millisoniyani nolga tushiramiz: forma vaqtni faqat HH:MM aniqligida
  // saqlaydi, shuning uchun taqqoslashda ular ham daqiqa aniqligida bo'lishi
  // kerak. Aks holda standart (eng erta) vaqt tanlangan bo'lsa ham "kamida
  // 1 soat keyin bo'lsin" tekshiruvi bir necha soniyaga yetmay yiqilardi.
  const minMoment = new Date(startRef.current.getTime() + 60 * 60 * 1000);
  minMoment.setSeconds(0, 0);
  const MIN_DATE = ymd(minMoment);
  const MAX_DATE = ymd(new Date(startRef.current.getTime() + 2 * 86400000));
  const minH = minMoment.getHours();
  const minM = minMoment.getMinutes();

  const [form, setForm] = useState<Form>({
    title: "",
    categoryId: "",
    description: "",
    loc: null,
    workersNeeded: 1,
    gender: "mixed",
    pricingType: "per_worker",
    priceAmount: "",
    startDate: MIN_DATE,
    workTimeFrom: hm(minMoment),
    // Faqat milliy qism ("90 123 45 67"). "+998" prefiksini PhoneInput
    // chizadi va u foydalanuvchi tomonidan o'zgartirilmaydi.
    contactPhone: "",
    images: [],
  });
  const [state, setState] = useState<"idle" | "ok" | "err">("idle");
  const [errMsg, setErrMsg] = useState("");
  const [uploading, setUploading] = useState(false);
  // Moderatsiya rad etgan e'lon — modal oynada sabab va ogohlantirish bilan
  // ko'rsatiladi (oddiy xato matni bunday holat uchun yetarli emas).
  const [modErr, setModErr] = useState<APIError | null>(null);

  const { data: cats } = useQuery<Category[]>({
    queryKey: ["categories"],
    queryFn: () => api.get<Category[]>("/api/categories"),
  });

  // Aloqa raqami ATAYLAB oldindan to'ldirilmaydi. Ilgari u profildagi raqam
  // bilan to'lardi, lekin e'londagi raqam ko'pincha boshqa bo'ladi (ishni
  // boshqa odam qabul qiladi, ish joyining raqami boshqa, ...). Tayyor turgan
  // raqam esa o'zgartirilmasdan yuborilib ketardi. Endi ish beruvchi bog'lanish
  // raqamini har safar o'zi yozadi.

  const create = useMutation({
    mutationFn: async () => {
      const workers = typeof form.workersNeeded === "number" && form.workersNeeded > 0 ? form.workersNeeded : 1;
      const price = typeof form.priceAmount === "number" ? form.priceAmount : 0;
      const body = {
        title: form.title,
        categoryId: form.categoryId,
        description: form.description,
        lat: form.loc?.lat || 0,
        lng: form.loc?.lng || 0,
        workersNeeded: workers,
        gender: form.gender,
        pricingType: price === 0 ? "negotiable" : form.pricingType,
        priceAmount: price,
        startDate: form.startDate,
        workTimeFrom: form.workTimeFrom,
        contactPhone: phoneDigits(form.contactPhone) ? fmtPhone(form.contactPhone) : "",
        images: form.images,
      };
      // E'lon darhol chop etiladi (qoralama bosqichi yo'q).
      if (!creationKey.current) {
        creationKey.current = Array.from(crypto.getRandomValues(new Uint8Array(12)), (byte) => byte.toString(16).padStart(2, "0")).join("");
      }
      return api.post<Elon>("/api/elons", body, { headers: { "Idempotency-Key": creationKey.current } });
    },
    onSuccess: (job) => { setState("ok"); onCreated?.(job); },
    onError: (e: any) => {
      if (isModerationError(e)) { setModErr(e); setState("idle"); return; }
      setErrMsg(e?.message || ""); setState("err");
    },
  });

  const live = useMemo(() => {
    const n = typeof form.workersNeeded === "number" ? form.workersNeeded : 0;
    const p = typeof form.priceAmount === "number" ? form.priceAmount : 0;
    if (!p) return { per: 0, total: 0, neg: true };
    if (form.pricingType === "total") return { per: n ? Math.floor(p / n) : 0, total: p, neg: false };
    return { per: p, total: p * n, neg: false };
  }, [form.workersNeeded, form.priceAmount, form.pricingType]);

  useEffect(() => { if (cats?.length && !form.categoryId) setForm((f) => ({ ...f, categoryId: cats[0].id })); }, [cats, form.categoryId]);

  // Tanlangan vaqtni eng erta ruxsat etilgan (hozir+1soat) dan oldin bo'lmasligini ta'minlaydi.
  function clampTime(dateStr: string, hh: number, mm: number): string {
    if (dateStr === MIN_DATE && (hh < minH || (hh === minH && mm < minM))) {
      hh = minH;
      mm = minM;
    }
    return `${pad(hh)}:${pad(mm)}`;
  }
  const [curH, curM] = (form.workTimeFrom || hm(minMoment)).split(":").map((x) => parseInt(x, 10) || 0);

  function submit() {
    if (uploading || create.isPending) return;
    if (!form.loc) { setErrMsg(t("Iltimos, ish joyini xaritadan belgilang.")); setState("err"); return; }
    const chosen = new Date(`${form.startDate}T${form.workTimeFrom || "00:00"}:00`);
    if (isNaN(chosen.getTime()) || chosen.getTime() < minMoment.getTime()) {
      setErrMsg(t("Ish boshlanish vaqti hozirgi vaqtdan kamida 1 soat keyin bo'lishi kerak."));
      setState("err"); return;
    }
    create.mutate();
  }

  if (state === "ok") return embedded ? null : <SuccessPage onMine={() => router.push("/my-elons")} onHome={() => router.push("/dashboard")} />;
  if (state === "err" && !embedded) return <ErrorPage msg={errMsg} onRetry={() => setState("idle")} onSupport={() => router.push("/feedback")} />;

  const Frame = embedded ? MiniAppFormFrame : Shell;

  return (
    <Frame wide>
      <div className="py-6 max-w-[760px] mx-auto w-full">
        <h1 className="text-[26px] font-black heading tracking-[-0.6px] leading-tight"><T>E'lon berish</T></h1>
        <p className="text-[13.5px] muted mt-1"><T>Vazifa tafsilotlarini kiriting va malakali ishchilarni toping.</T></p>

        <form
          onSubmit={(e) => { e.preventDefault(); submit(); }}
          className="card p-6 sm:p-7 grid gap-5 mt-5"
        >
          <Field label="Vazifa nomi *">
            <input className="input" required value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder={t("Masalan: Hovlini tozalash")} />
          </Field>

          <Field label="Kategoriya *">
            <div className="flex flex-wrap gap-2">
              {(cats || []).map((c) => (
                <button key={c.id} type="button"
                  onClick={() => setForm({ ...form, categoryId: c.id })}
                  className={`chip ${form.categoryId === c.id ? "chip-active" : ""}`}>
                  <CategoryIcon icon={c.icon} name={c.name} className="h-4 w-4" />
                  <T>{c.name}</T>
                </button>
              ))}
            </div>
          </Field>

          <Field label="Vazifa tavsifi *">
            <textarea className="input min-h-[110px]" required value={form.description}
                      onChange={(e) => setForm({ ...form, description: e.target.value })}
                      placeholder={t("Vazifa haqida batafsilroq ma'lumot bering…")} />
          </Field>

          {/* Ish joyi — xaritadan tanlanadi. Viloyat/tuman avtomatik aniqlanadi. */}
          <Field label="Ish joyi (xaritadan belgilang) *">
            <MapPicker value={form.loc} onChange={(loc) => setForm((f) => ({ ...f, loc }))} />
          </Field>

          {/* Kim kerak — Figma: uchta segment tugma */}
          <Field label="Kim kerak?">
            <div className="grid grid-cols-3 gap-2.5">
              {GENDER_OPTIONS.map((g) => {
                const on = form.gender === g;
                return (
                  <button key={g} type="button" onClick={() => setForm({ ...form, gender: g })}
                    className="rounded-lg border py-3 text-[13.5px] font-semibold inline-flex items-center justify-center gap-2 transition"
                    style={on
                      ? { borderColor: "var(--brand)", background: "var(--brand-soft)", color: "var(--brand)", borderWidth: 1.5 }
                      : { borderColor: "var(--border-strong)", background: "var(--bg-subtle)", color: "var(--text-muted)" }}>
                    <User2 size={15} /><T>{GENDER_LABEL[g]}</T>
                  </button>
                );
              })}
            </div>
            <div className="mt-2 text-[12px] subtle"><T>«Aralash» tanlansa, e'lon barcha ishchilarga ko'rinadi.</T></div>
          </Field>

          {/* Ish haqi + ishchilar soni */}
          <div className="grid sm:grid-cols-2 gap-4">
            <Field label="Ish haqi (UZS)">
              <input
                type="text" inputMode="numeric" className="input"
                placeholder={t("Masalan: 150 000")}
                value={form.priceAmount === "" ? "" : fmtThousands(String(form.priceAmount))}
                onChange={(e) => {
                  const digits = onlyDigits(e.target.value);
                  setForm({ ...form, priceAmount: digits === "" ? "" : Number(digits) });
                }}
              />
            </Field>
            <Field label="Ishchilar soni *">
              <div className="input flex items-center justify-between !py-1.5">
                <button type="button" aria-label="−"
                  onClick={() => setForm((f) => ({ ...f, workersNeeded: Math.max(1, (typeof f.workersNeeded === "number" ? f.workersNeeded : 1) - 1) }))}
                  className="grid h-8 w-8 place-items-center rounded-lg text-lg font-bold transition hover:bg-[color:var(--brand-soft)]"
                  style={{ color: "var(--brand)" }}>−</button>
                <span className="text-[15px] font-bold heading">{form.workersNeeded || 1}</span>
                <button type="button" aria-label="+"
                  onClick={() => setForm((f) => ({ ...f, workersNeeded: (typeof f.workersNeeded === "number" ? f.workersNeeded : 1) + 1 }))}
                  className="grid h-8 w-8 place-items-center rounded-lg text-lg font-bold transition hover:bg-[color:var(--brand-soft)]"
                  style={{ color: "var(--brand)" }}>+</button>
              </div>
            </Field>
          </div>

          {/* Narx turi — Figma: ikkita radio karta */}
          <Field label="Kiritilgan summa nimani bildiradi?">
            <div className="grid sm:grid-cols-2 gap-2.5">
              {([
                ["per_worker", "Har bir ishchi uchun", `Har bir ishchiga alohida ${fmtSum(live.per || 0)} so'm to'lanadi`],
                ["total", "Umumiy summa", `${fmtSum(live.total || 0)} so'm barcha ishchilarga bo'linadi`],
              ] as const).map(([val, title, hint]) => {
                const on = form.pricingType === val;
                return (
                  <button key={val} type="button" onClick={() => setForm({ ...form, pricingType: val })}
                    className="rounded-lg border p-3.5 text-left flex gap-2.5 transition"
                    style={on
                      ? { borderColor: "var(--brand)", background: "var(--brand-soft)", borderWidth: 1.5 }
                      : { borderColor: "var(--border-strong)", background: "var(--bg-subtle)" }}>
                    <span className="mt-0.5 grid h-4 w-4 shrink-0 place-items-center rounded-full border-2"
                          style={{ borderColor: on ? "var(--brand)" : "var(--border-strong)" }}>
                      {on && <span className="h-2 w-2 rounded-full" style={{ background: "var(--brand)" }} />}
                    </span>
                    <span className="min-w-0">
                      <span className="block text-[13.5px] font-bold" style={{ color: on ? "var(--brand)" : "var(--text)" }}>
                        <T>{title}</T>
                      </span>
                      <span className="block text-[11.5px] subtle mt-0.5">{hint}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </Field>

          {live.neg ? (
            <div className="rounded-lg p-3.5 text-[12.5px] leading-relaxed"
                 style={{ background: "var(--accent-soft)", color: "var(--accent-text)" }}>
              <T>Ish haqi maydonini bo'sh qoldirsangiz, e'lon avtomatik «Kelishiladi» sifatida joylanadi.</T>
            </div>
          ) : (
            <div className="rounded-lg p-3.5 text-[12.5px] font-semibold"
                 style={{ background: "var(--brand-soft)", color: "var(--brand)" }}>
              <T>Jami to'lanadigan summa</T>: {fmtSum(live.total)} so'm ({fmtSum(live.per)} × {form.workersNeeded || 1} <T>ishchi</T>)
            </div>
          )}

          <div className="grid sm:grid-cols-2 gap-4">
          <Field label="Boshlanish sanasi *">
            <input required type="date" min={MIN_DATE} max={MAX_DATE} className="input" value={form.startDate}
              onChange={(e) => { const d = e.target.value; setForm({ ...form, startDate: d, workTimeFrom: clampTime(d, curH, curM) }); }} />
            <div className="mt-1 text-xs muted"><T>Bugundan 3 kun ichida</T></div>
          </Field>
          <Field label="Boshlanish vaqti (24 soat) *">
            <div className="flex items-center gap-2">
              <select className="input" value={curH}
                onChange={(e) => setForm({ ...form, workTimeFrom: clampTime(form.startDate, parseInt(e.target.value, 10), curM) })}>
                {Array.from({ length: 24 }).map((_, h) => (
                  <option key={h} value={h} disabled={form.startDate === MIN_DATE && h < minH}>{pad(h)}</option>
                ))}
              </select>
              <span className="font-semibold">:</span>
              <select className="input" value={curM}
                onChange={(e) => setForm({ ...form, workTimeFrom: clampTime(form.startDate, curH, parseInt(e.target.value, 10)) })}>
                {Array.from({ length: 60 }).map((_, m) => (
                  <option key={m} value={m} disabled={form.startDate === MIN_DATE && curH === minH && m < minM}>{pad(m)}</option>
                ))}
              </select>
            </div>
            <div className="mt-1 text-xs muted"><T>Kamida hozirgi vaqtdan 1 soat keyin (24 soatlik)</T></div>
          </Field>
          </div>

          <Field label="Aloqa telefon raqami *">
            <PhoneInput
              required
              className="sm:max-w-[280px]"
              value={form.contactPhone}
              onChange={(contactPhone) => setForm({ ...form, contactPhone })}
            />
          </Field>

          <Field label="Rasmlar (ixtiyoriy)">
            <MultiImageUploader value={form.images} onChange={(images) => setForm((current) => ({ ...current, images }))} onBusyChange={setUploading} disabled={create.isPending} max={6} />
            <div className="mt-1.5 text-[12px] subtle">JPG / PNG / WebP, har biri 8MB gacha. Maks 6 ta rasm.</div>
          </Field>

          <div className="divider" />

          {embedded && state === "err" && <p role="alert" className="text-sm text-danger">{errMsg}</p>}

          <div className="flex flex-wrap gap-2.5">
            <button className="btn btn-primary flex-1 !py-3.5" disabled={create.isPending || uploading}>
              {uploading ? "Rasmlar yuklanmoqda…" : create.isPending ? "Joylashtirilmoqda…" : <T>E'lonni joylashtirish</T>}
            </button>
            <button type="button" className="btn btn-outline" onClick={() => onCancel ? onCancel() : router.back()}><T>Bekor qilish</T></button>
          </div>
        </form>
      </div>
      <ModerationModal error={modErr} onClose={() => setModErr(null)} />
    </Frame>
  );
}

function MiniAppFormFrame({ children }: { children: React.ReactNode; wide?: boolean }) {
  return <main className="px-3 pb-6 sm:px-5">{children}</main>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-[13px] font-bold heading"><T>{label}</T></span>
      <div className="mt-1.5">{children}</div>
    </label>
  );
}

function SuccessPage({ onMine, onHome }: { onMine: () => void; onHome: () => void }) {
  return (
    <Shell title="E'lon joylashtirildi">
      <div className="card p-10 text-center">
        <CheckCircle2 size={56} className="mx-auto text-success" />
        <h2 className="text-xl font-bold mt-3"><T>E'loningiz muvaffaqiyatli joylashtirildi!</T></h2>
        <p className="text-[color:var(--text-muted)] mt-1"><T>Endi ishchilar arizalarini kuting.</T></p>
        <div className="mt-5 flex justify-center gap-2">
          <button onClick={onMine} className="btn-primary"><T>Mening e'lonlarimga o'tish</T></button>
          <button onClick={onHome} className="btn-secondary"><T>Asosiy sahifaga qaytish</T></button>
        </div>
      </div>
    </Shell>
  );
}

function ErrorPage({ msg, onRetry, onSupport }: { msg?: string; onRetry: () => void; onSupport: () => void }) {
  return (
    <Shell title="Xatolik">
      <div className="card p-10 text-center">
        <AlertTriangle size={56} className="mx-auto text-danger" />
        <h2 className="text-xl font-bold mt-3"><T>E'lonni joylashtirishda xatolik yuz berdi</T></h2>
        <p className="text-[color:var(--text-muted)] mt-1">{msg ? msg : <T>Iltimos, qayta urinib ko'ring.</T>}</p>
        <div className="mt-5 flex justify-center gap-2">
          <button onClick={onRetry} className="btn-primary"><T>Qayta urinib ko'rish</T></button>
          <button onClick={onSupport} className="btn-secondary"><T>Yordamga murojaat qilish</T></button>
        </div>
      </div>
    </Shell>
  );
}
