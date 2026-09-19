"use client";
import { useRef, useState } from "react";
import { Camera, Loader2, Trash2 } from "lucide-react";
import { uploadFile, deleteUploaded, UploadKind } from "@/lib/upload";
import { APIError } from "@/lib/api";
import { ModerationModal, isModerationError } from "@/components/ModerationModal";
import { Avatar } from "./Avatar";
import { uploadPreviewURL } from "@/lib/miniapp";

interface AvatarUploaderProps {
  value?: string;             // current URL
  name?: string;              // user name for fallback initials
  onChange: (url: string | undefined) => void;
  kind?: UploadKind;          // default: avatar
  scope?: string;
}

/**
 * Round avatar uploader — click or drop a file onto the circle.
 * Calls onChange(url) on success, onChange(undefined) when cleared.
 */
export function AvatarUploader({ value, name, onChange, kind = "avatar", scope }: AvatarUploaderProps) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [pct, setPct] = useState(0);
  const [err, setErr] = useState("");
  // Moderatsiya rad etgan rasm — modal oynada sabab va ogohlantirish bilan
  // ko'rsatiladi (kichik qizil matn bunday jiddiy holat uchun yetarli emas).
  const [modErr, setModErr] = useState<APIError | null>(null);

  async function handle(f: File) {
    if (!f.type.startsWith("image/")) { setErr("Faqat rasm qabul qilinadi"); return; }
    if (f.size > 5 * 1024 * 1024) { setErr("Maks 5MB"); return; }
    setErr(""); setBusy(true); setPct(0);
    try {
      const old = value;
      const res = await uploadFile(f, kind, { scope, onProgress: setPct });
      onChange(res.url);
      if (old) deleteUploaded({ url: old }).catch(() => {});
    } catch (e: any) {
      if (isModerationError(e)) setModErr(e);
      else setErr(e?.message || "Xatolik");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div
        onClick={() => !busy && fileRef.current?.click()}
        onDragOver={(e) => { e.preventDefault(); }}
        onDrop={(e) => { e.preventDefault(); const f = e.dataTransfer.files?.[0]; if (f) handle(f); }}
        className="relative inline-grid place-items-center cursor-pointer group rounded-full"
        title="Rasm yuklash"
      >
        <Avatar size="xl" name={name} src={value} />
        <div className="absolute inset-0 rounded-full bg-black/40 opacity-0 group-hover:opacity-100 transition grid place-items-center text-white">
          {busy ? <Loader2 size={20} className="animate-spin" /> : <Camera size={18} />}
        </div>
        {busy && pct > 0 && (
          <div className="absolute -bottom-1 left-0 right-0 h-1 rounded-full bg-[color:var(--border)] overflow-hidden">
            <div className="h-full bg-brand-navy transition-all" style={{ width: `${pct}%` }} />
          </div>
        )}
      </div>
      <input
        ref={fileRef}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className="hidden"
        onChange={(e) => { const f = e.target.files?.[0]; if (f) handle(f); e.target.value = ""; }}
      />
      <div className="mt-2 flex items-center gap-2 text-xs">
        <button type="button" onClick={() => fileRef.current?.click()} disabled={busy} className="btn-secondary btn-sm">
          {busy ? "Yuklanmoqda…" : "Rasm yuklash"}
        </button>
        {value && (
          <button
            type="button"
            onClick={() => { const old = value; onChange(undefined); deleteUploaded({ url: old! }).catch(() => {}); }}
            className="btn-ghost btn-sm text-danger"
          >
            <Trash2 size={12} /> O'chirish
          </button>
        )}
      </div>
      {err && <p className="text-xs text-danger mt-1">{err}</p>}
      <ModerationModal error={modErr} onClose={() => setModErr(null)} />
    </div>
  );
}

interface MultiImageProps {
  value: string[];
  onChange: (urls: string[]) => void;
  onBusyChange?: (busy: boolean) => void;
  disabled?: boolean;
  max?: number;
  kind?: UploadKind;
  scope?: string;
}

/** Multiple-image uploader used in elon create/edit. */
export function MultiImageUploader({ value, onChange, onBusyChange, disabled = false, max = 6, kind = "elon", scope }: MultiImageProps) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [modErr, setModErr] = useState<APIError | null>(null);

  async function handle(files: FileList | null) {
    if (!files || busy || disabled) return;
    setErr("");
    const remaining = max - value.length;
    if (remaining <= 0) { setErr(`Maksimal ${max} ta rasm qo'shish mumkin.`); return; }
    const arr = Array.from(files).slice(0, remaining);
    setBusy(true);
    onBusyChange?.(true);
    const next = [...value];
    for (const f of arr) {
      if (!f.type.startsWith("image/")) { setErr("Faqat rasm fayllari qabul qilinadi (JPG, PNG, WebP)."); continue; }
      if (f.size > 8 * 1024 * 1024) { setErr("Har bir rasm hajmi 8MB dan oshmasligi kerak."); continue; }
      try {
        const r = await uploadFile(f, kind, { scope });
        next.push(r.url);
        onChange([...next]);
      } catch (e: any) {
        if (isModerationError(e)) setModErr(e);
        else setErr(e?.message || "Rasm yuklashda xatolik yuz berdi.");
      }
    }
    setBusy(false);
    onBusyChange?.(false);
  }

  function remove(url: string) {
    onChange(value.filter((u) => u !== url));
    deleteUploaded({ url }).catch(() => {});
  }

  return (
    <div>
      <div className="grid grid-cols-3 sm:grid-cols-4 gap-2">
        {value.map((u) => (
          <div key={u} className="relative aspect-square rounded-xl overflow-hidden border" style={{ borderColor: "var(--border)" }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={uploadPreviewURL(u)} alt="" className="w-full h-full object-cover" />
            <button
              type="button"
              disabled={busy || disabled}
              aria-label="Rasmni o'chirish"
              onClick={() => remove(u)}
              className="absolute top-1 right-1 grid place-items-center h-6 w-6 rounded-full bg-black/60 text-white hover:bg-danger transition"
            >
              <Trash2 size={12} />
            </button>
          </div>
        ))}
        {value.length < max && (
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            disabled={busy || disabled}
            aria-label="Rasm qo'shish"
            className="aspect-square rounded-xl border border-dashed grid place-items-center muted hover:bg-[color:var(--bg-subtle)] transition"
            style={{ borderColor: "var(--border-strong)" }}
          >
            {busy ? <Loader2 size={18} className="animate-spin" /> : <Camera size={18} />}
          </button>
        )}
      </div>
      <input
        ref={fileRef}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        multiple
        className="hidden"
        onChange={(e) => { handle(e.target.files); e.target.value = ""; }}
      />
      {err && <p className="text-xs text-danger mt-1">{err}</p>}
      <ModerationModal error={modErr} onClose={() => setModErr(null)} />
    </div>
  );
}
