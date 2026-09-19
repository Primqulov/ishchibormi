"use client";
import { userAPIURL, getAccess } from "@/lib/api";

export type UploadKind = "avatar" | "elon";

export interface UploadedFile {
  key: string;
  url: string;
}

export interface UploadOpts {
  scope?: string;                   // e.g. an elon id
  onProgress?: (pct: number) => void;
}

/**
 * Uploads a single file to the backend, which streams it to S3.
 * Returns the public URL + S3 key. Throws on failure.
 */
export function uploadFile(file: File, kind: UploadKind, opts: UploadOpts = {}): Promise<UploadedFile> {
  return new Promise((resolve, reject) => {
    const token = getAccess();
    if (!token) return reject(new Error("Tizimga kirilmagan"));

    const fd = new FormData();
    fd.append("file", file, file.name);

    const url = new URL(userAPIURL("/api/uploads"), window.location.origin);
    url.searchParams.set("kind", kind);
    if (opts.scope) url.searchParams.set("scope", opts.scope);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", url.toString(), true);
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);

    if (opts.onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) opts.onProgress!(Math.round((e.loaded / e.total) * 100));
      };
    }
    xhr.onerror = () =>
      reject(
        new Error(
          typeof navigator !== "undefined" && navigator.onLine === false
            ? "Internet aloqasi yo'q. Tarmoqni tekshirib, qayta urinib ko'ring."
            : "Server vaqtincha ishlamayapti. Birozdan so'ng qayta urinib ko'ring."
        )
      );
    xhr.onload = () => {
      try {
        const data = xhr.responseText ? JSON.parse(xhr.responseText) : {};
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve({ key: data.key, url: data.url });
        } else if (xhr.status >= 500) {
          reject(
            new Error(
              "Serverda vaqtinchalik xatolik bor. Birozdan so'ng qayta urinib ko'ring."
            )
          );
        } else {
          // Error o'rniga UploadError: moderatsiya rad etganda `code` va
          // `details` kerak bo'ladi (modal oyna sabab va ogohlantirishni
          // alohida ko'rsatadi). Oddiy Error ularni yo'qotardi.
          reject(
            new UploadError(
              data?.error?.code || "upload_failed",
              data?.error?.message ||
                "Faylni yuklab bo'lmadi. Ma'lumotlarni tekshirib, qayta urinib ko'ring.",
              data?.error?.details
            )
          );
        }
      } catch (e) {
        reject(e as Error);
      }
    };
    xhr.send(fd);
  });
}

/** Tells the backend to remove an uploaded object (best-effort). */
export async function deleteUploaded(urlOrKey: { url?: string; key?: string }): Promise<void> {
  const token = getAccess();
  if (!token) return;
  const u = new URL(userAPIURL("/api/uploads"), window.location.origin);
  if (urlOrKey.url) u.searchParams.set("url", urlOrKey.url);
  if (urlOrKey.key) u.searchParams.set("key", urlOrKey.key);
  await fetch(u.toString(), {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
}

/**
 * Fayl yuklashda backend qaytargan xato — `message` dan tashqari `code` va
 * `details` ni ham saqlaydi. Error'dan meros olgani uchun mavjud
 * `e?.message` chaqiruvlari o'zgarishsiz ishlayveradi.
 */
export class UploadError extends Error {
  code: string;
  details?: Record<string, any>;

  constructor(code: string, message: string, details?: Record<string, any>) {
    super(message);
    this.name = "UploadError";
    this.code = code;
    this.details = details;
  }
}
