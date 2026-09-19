"use client";

import { isMiniAppPage } from "@/lib/miniapp";
import { fetchMiniApp, isTransientMiniAppStatus } from "@/lib/miniapp-transport";

export const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";
export const WS_BASE = process.env.NEXT_PUBLIC_WS_BASE || "ws://localhost:8080";

/**
 * Admin so'rovlari qaysi manzilga ketishi.
 *
 * Productionda — SAME-ORIGIN (bo'sh prefiks). Bu bezak emas, shart: admin
 * sessiyasini uzoq tutib turadigan refresh token HttpOnly cookie'da yuradi va
 * brauzer bunday cookie'ni faqat o'z originiga ishonchli yuboradi. Panel esa
 * faqat boshqaruv subdomenida ochiladi (middleware.ts) va u yerda Caddy
 * `/api/*` ni backendga o'zi uzatadi — ya'ni "same-origin" har doim to'g'ri.
 *
 * Xost nomi bu yerda ATAYLAB yozilmagan: bitta build ommaviy saytga ham
 * xizmat qiladi, ya'ni nom brauzerga yuklanadigan JS ichiga tushib, maxfiy
 * bo'lmay qolardi.
 *
 * Lokal ishlashda (`next dev` `/api/*` ni uzatmaydi) — odatdagi API_BASE.
 */
export function adminBase(): string {
  return process.env.NODE_ENV === "production" ? "" : API_BASE;
}

const ACCESS_KEY = "ib-access";
const MINIAPP_ACCESS_KEY = "ib-miniapp-access";

export function userAPIURL(path: string): string {
  return isMiniAppPage() ? `/miniapp/api${path.replace(/^\/api(?=\/)/, "")}` : `${API_BASE}${path}`;
}
const ADMIN_KEY = "ib-admin";
// Legacy key: the refresh token used to be persisted here. The web app never
// calls the refresh endpoint — the access token TTL alone defines the session —
// so a long-lived refresh token sitting in localStorage was pure XSS attack
// surface (one XSS meant weeks of account access). We no longer store it and
// actively purge any leftover value from existing users' browsers via setAccess.
const LEGACY_REFRESH_KEY = "ib-refresh";

export function getAccess(): string | null {
  if (typeof window === "undefined") return null;
  if (isMiniAppPage()) return sessionStorage.getItem(MINIAPP_ACCESS_KEY);
  return localStorage.getItem(ACCESS_KEY);
}
export function setAccess(t: string | null) {
  if (typeof window === "undefined") return;
  if (isMiniAppPage()) {
    if (t) sessionStorage.setItem(MINIAPP_ACCESS_KEY, t);
    else sessionStorage.removeItem(MINIAPP_ACCESS_KEY);
    return;
  }
  if (t) localStorage.setItem(ACCESS_KEY, t);
  else localStorage.removeItem(ACCESS_KEY);
  // Whenever the user's auth state changes (login, logout, 401), drop any
  // legacy refresh token that may still be stored from an older build.
  localStorage.removeItem(LEGACY_REFRESH_KEY);
}

export function setAdminToken(t: string | null) {
  if (typeof window === "undefined") return;
  // Admin sessions are deliberately tab-scoped and disappear when the browser
  // closes. This limits persistence of a privileged bearer token. Purge the
  // legacy localStorage copy during migration.
  localStorage.removeItem(ADMIN_KEY);
  if (t) sessionStorage.setItem(ADMIN_KEY, t);
  else sessionStorage.removeItem(ADMIN_KEY);
}
export function getAdminToken(): string | null {
  if (typeof window === "undefined") return null;
  localStorage.removeItem(ADMIN_KEY);
  return sessionStorage.getItem(ADMIN_KEY);
}

/**
 * Admin access tokenini yangilaydi.
 *
 * Access token ataylab qisqa umr ko'radi (30 daqiqa) — o'g'irlangani tez
 * o'lishi uchun. Uni yangilab turadigan refresh token esa HttpOnly cookie'da
 * bo'lgani uchun bu yerdagi kod uni ko'rmaydi ham: brauzer so'rovga o'zi
 * qo'shadi. Backend 3 kunlik "foydalanilmasa chiqarish" oynasini shu yerda
 * tekshiradi (apps/api/internal/admin/refresh.go).
 *
 * Bitta uchuvchi so'rov: sahifada bir vaqtda bir necha so'rov 401 olsa,
 * hammasi shu bitta yangilashni kutadi — aks holda ular bir-birining
 * tokenini almashtirib yuborardi (refresh token har chaqiruvda aylanadi).
 */
let adminRefreshInFlight: Promise<boolean> | null = null;

async function runAdminRefresh(): Promise<boolean> {
  try {
    const res = await fetch(`${adminBase()}/api/admin/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Client-Platform": "web" },
      // Cookie same-origin'da avtomatik ketadi; lokal ishlashda (3000 -> 8080)
      // esa faqat shu bayroq bilan yuboriladi.
      credentials: "include",
      body: "{}",
    });
    if (!res.ok) return false;
    const data = await res.json();
    if (!data?.accessToken) return false;
    setAdminToken(data.accessToken);
    return true;
  } catch {
    return false;
  }
}

export function adminRefresh(): Promise<boolean> {
  if (!adminRefreshInFlight) {
    adminRefreshInFlight = runAdminRefresh().finally(() => {
      adminRefreshInFlight = null;
    });
  }
  return adminRefreshInFlight;
}

// downloadAdminCsv triggers a browser download of an admin CSV export. The admin
// JWT goes in the Authorization header (fetch + blob), never in the URL —
// query-string tokens end up in proxy access logs and browser history.
export async function downloadAdminCsv(path: string, params?: URLSearchParams) {
  if (typeof document === "undefined") return;
  const qs = params && params.toString() ? `?${params}` : "";
  try {
    const fetchCsv = () =>
      fetch(`${adminBase()}${path}${qs}`, {
        headers: { Authorization: `Bearer ${getAdminToken() || ""}` },
        credentials: "include",
      });
    let res = await fetchCsv();
    // Eksport odatda uzoq ochiq turgan sahifadan bosiladi — aynan shu paytda
    // access token eskirgan bo'lishi mumkin. Bir marta yangilab qayta uramiz.
    if (res.status === 401 && (await adminRefresh())) res = await fetchCsv();
    if (!res.ok) throw new Error(`export failed: ${res.status}`);
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    // /api/admin/export/users.csv -> users.csv
    a.download = path.split("/").pop() || "export.csv";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch (e) {
    console.error("CSV export:", e);
    alert("Eksport muvaffaqiyatsiz. Qayta urinib ko'ring.");
  }
}

// getAdminRole decodes the role claim from the stored admin JWT so the UI can
// hide sections a role can't use (RBAC is still enforced server-side).
export function getAdminRole(): string | null {
  const t = getAdminToken();
  if (!t) return null;
  try {
    const part = t.split(".")[1];
    const json = atob(part.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json).role || null;
  } catch {
    return null;
  }
}

export interface APIError {
  code: string;
  message: string;
  /**
   * Backend'ning strukturali qo'shimchasi (error.details). Moderatsiya rad
   * etganda modal oyna shundan sabab va ogohlantirishni alohida oladi:
   * { reason, warning?, strikes?, strikeLimit?, bannedUntil? }
   */
  details?: Record<string, any>;
}

function connectionError(): APIError {
  const offline =
    typeof navigator !== "undefined" && navigator.onLine === false;
  return offline
    ? {
        code: "offline",
        message:
          "Internet aloqasi yo'q. Tarmoqni tekshirib, qayta urinib ko'ring.",
      }
    : {
        code: "server_unavailable",
        message:
          "Server vaqtincha ishlamayapti. Internetingiz ishlayapti — birozdan so'ng qayta urinib ko'ring.",
      };
}

function responseError(status: number, data: any): APIError {
  if (isMiniAppPage() && isTransientMiniAppStatus(status)) {
    return { code: "miniapp_connection_unavailable", message: "Ulanish vaqtincha uzildi. Birozdan so'ng qayta urinib ko'ring." };
  }
  if (status >= 500) {
    return {
      code: "server_error",
      message:
        "Serverda vaqtinchalik xatolik bor. Birozdan so'ng qayta urinib ko'ring.",
    };
  }

  const backendError = data?.error;
  if (
    backendError &&
    typeof backendError.code === "string" &&
    typeof backendError.message === "string"
  ) {
    // Business errors keep their stable code so forms can show a helpful local
    // message. Callers must not display raw technical response bodies.
    return backendError as APIError;
  }

  return {
    code: "request_failed",
    message:
      "So'rovni bajarib bo'lmadi. Ma'lumotlarni tekshirib, qayta urinib ko'ring.",
  };
}

async function request<T>(
  path: string,
  // _retried — 401 dan keyingi bitta qayta urinish belgisi. Usiz token
  // yangilanmaydigan holatda so'rov cheksiz aylanib qolardi.
  opts: RequestInit & { auth?: "user" | "admin" | "none"; _retried?: boolean } = {}
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    // Foydalanuvchi qaysi klientdan foydalanayotganini backend shu
    // sarlavhadan biladi (admin panelidagi platforma statistikasi).
    // User-Agent yetarli emas: uni kengaytmalar o'zgartiradi va mobil
    // ilovadagi WebView ham brauzer UA'sini yuboradi.
    "X-Client-Platform": "web",
  };
  const auth = opts.auth ?? "user";
  if (auth === "user") {
    const t = getAccess();
    if (t) headers["Authorization"] = `Bearer ${t}`;
  } else if (auth === "admin") {
    const t = getAdminToken();
    if (t) headers["Authorization"] = `Bearer ${t}`;
  }
  Object.assign(headers, opts.headers || {});

  // fetch() server topilmaganda (backend o'chiq, CORS, oflayn) xom
  // `TypeError: Failed to fetch` tashlaydi — bu Next.js dev'da "Unhandled
  // Runtime Error" overlay'i bo'lib chiqadi. navigator.onLine orqali haqiqiy
  // oflayn holatni online qurilmadagi backend nosozligidan ajratamiz.
  // Manzil YO'L bo'yicha tanlanadi, `auth` rejimi bo'yicha EMAS.
  //
  // Sabab: /api/admin/login ataylab `auth: "none"` bilan chaqiriladi — hali
  // token yo'q. Qaror rejimga bog'langanida aynan shu bitta so'rov ommaviy
  // domenga ketardi, u yerda esa admin API 404. Bundan ham yomoni, u
  // cross-origin bo'lgani uchun brauzer CORS tekshiruvidayoq to'sardi va
  // fetch xato tashlardi — foydalanuvchi buni "Server vaqtincha
  // ishlamayapti" degan xabar sifatida ko'rardi, ya'ni sabab butunlay
  // yashirin qolardi.
  //
  // Yo'l bo'yicha tanlash o'z-o'zidan to'g'ri: /api/admin/* faqat boshqaruv
  // subdomenida yashaydi, kim qanday chaqirishidan qat'i nazar.
  const isAdminPath = path.startsWith("/api/admin");
  let res: Response;
  let text: string;
  try {
    const url = isAdminPath ? `${adminBase()}${path}` : userAPIURL(path);
    const init: RequestInit = {
      ...opts,
      headers,
      // Admin sessiyasining refresh cookie'si so'rov bilan birga ketsin —
      // login javobi aynan shu cookie'ni o'rnatadi. Foydalanuvchi oqimida
      // cookie umuman yo'q (faqat Bearer token), u yerda o'zgarish yo'q.
      ...(isAdminPath ? { credentials: "include" as RequestCredentials } : {}),
    };
    if (!isAdminPath && isMiniAppPage()) {
      ({ response: res, text } = await fetchMiniApp(url, init));
    } else {
      res = await fetch(url, init);
      text = await res.text();
    }
  } catch {
    throw connectionError();
  }

  // Javob JSON bo'lmasligi mumkin (masalan proksi 502 HTML sahifasi) — parse
  // xatosi butun so'rovni yiqitmasin.
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }
  if (!res.ok) {
    // Sessiya tugagan yoki token yaroqsiz (401) — saqlangan foydalanuvchi
    // tokenlarini tozalaymiz. Shunda ilova "kirgan" holatda qotib qolmaydi
    // va foydalanuvchi qaytadan login qilishga yo'naltiriladi.
    //
    // 403 account_disabled ham xuddi shunday: hisob boshqa qurilmada (masalan
    // ilovada) o'chirilgan yoki bloklangan bo'lsa, JWT hali "yaroqli" ko'rinadi
    // — faqat backend uni rad etadi. Tokenni tozalamasak brauzer o'zini
    // "kirgan" deb hisoblab qolaveradi: landing'dagi "Kirish" tugmasi /login
    // o'rniga /dashboard'ga olib boradi va Shell'ning redirect qorovuli
    // (getAccess() hali ham to'la) hech qachon ishlamaydi.
    const disabled =
      res.status === 403 && data?.error?.code === "account_disabled";
    if ((res.status === 401 || disabled) && auth === "user") {
      setAccess(null);
    }
    // Admin JWT ham xuddi shunday eskirishi mumkin (masalan lokal backend
    // qayta ishga tushganda boshqa secret bilan). Token shunchaki mavjudligi
    // admin kirganini anglatmaydi: 401 da uni darhol tozalab, qayta login
    // qilishga yo'naltiramiz. Aks holda panel har bir amalni "invalid token"
    // bilan rad etib, foydalanuvchini kirgan holatda qotirib qo'yadi.
    if (res.status === 401 && auth === "admin") {
      // 401 "sessiya tugadi" degani EMAS. Access token atigi 30 daqiqa
      // yashaydi, shuning uchun avval uni jimgina yangilab ko'ramiz: 3 kunlik
      // oyna ochiq bo'lsa admin buni umuman sezmaydi va shu so'rov qaytadan
      // yuboriladi. Faqat yangilash ham rad etilsa — haqiqatan chiqaramiz.
      if (!opts._retried && (await adminRefresh())) {
        return request<T>(path, { ...opts, _retried: true });
      }
      setAdminToken(null);
      if (typeof window !== "undefined" && window.location.pathname !== "/admin/login") {
        window.location.replace("/admin/login?session=expired");
      }
    }
    throw responseError(res.status, data);
  }
  return data as T;
}

export const api = {
  get: <T>(p: string, opts?: any) => request<T>(p, { ...(opts || {}), method: "GET" }),
  post: <T>(p: string, body?: any, opts?: any) =>
    request<T>(p, { ...(opts || {}), method: "POST", body: body !== undefined ? JSON.stringify(body) : undefined }),
  patch: <T>(p: string, body?: any, opts?: any) =>
    request<T>(p, { ...(opts || {}), method: "PATCH", body: body !== undefined ? JSON.stringify(body) : undefined }),
  put: <T>(p: string, body?: any, opts?: any) =>
    request<T>(p, { ...(opts || {}), method: "PUT", body: body !== undefined ? JSON.stringify(body) : undefined }),
  delete: <T>(p: string, opts?: any) => request<T>(p, { ...(opts || {}), method: "DELETE" }),
};

// ----- domain types -----
export type ID = string;

/**
 * Backend biladigan klientlar (Go: httpx.Platform*). Ro'yxat YOPIQ —
 * backend klient yuborgan qiymatni shu to'plamga keltiradi.
 */
export type ClientPlatform = "web" | "android" | "ios" | "unknown";

/** Panelda ko'rsatiladigan tartib — hisobotlarda ham shu ketma-ketlik. */
export const CLIENT_PLATFORMS: ClientPlatform[] = ["web", "android", "ios", "unknown"];

const PLATFORM_LABELS: Record<ClientPlatform, string> = {
  web: "Veb",
  android: "Android",
  ios: "iOS",
  // "Noma'lum" — bu funksiyadan oldin ro'yxatdan o'tganlar va sarlavha
  // yubormaydigan eski ilova versiyalari. Ataylab ko'rsatiladi: yashirilsa
  // ustunlar yig'indisi jami foydalanuvchiga teng kelmasdi.
  unknown: "Noma'lum",
};

/**
 * Veb klient qaysi qurilma OS'ida ochilgani (Go: httpx.Device*). Ro'yxat
 * YOPIQ — qiymatni backend User-Agent'dan aniqlab, shu to'plamga keltiradi.
 */
export type ClientDevice =
  | "android"
  | "ios"
  | "windows"
  | "macos"
  | "linux"
  | "chromeos"
  /**
   * ESKI qiymat: aniq OS yozila boshlangunga qadar har qanday stol
   * kompyuteri shunday saqlangan. Yangi yozuvlarda chiqmaydi, lekin
   * bazadagi eskilarini o'qish uchun turda qoladi.
   */
  | "desktop";

/**
 * Qurilmaning platforma yorlig'iga qo'shiladigan qismi.
 *
 * `desktop` ATAYLAB yo'q: u aniq OS emas, shunchaki "mobil emas" degani,
 * ya'ni "Veb Desktop" qatorga hech qanday ma'lumot qo'shmasdi. Bunday eski
 * yozuvlar qo'shimchasiz "Veb" bo'lib ko'rinadi.
 */
const DEVICE_SUFFIXES: Partial<Record<ClientDevice, string>> = {
  android: "Android",
  ios: "iOS",
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
  chromeos: "ChromeOS",
};

/**
 * Platforma kodini panelda ko'rsatiladigan nomga aylantiradi.
 *
 * Ikkinchi argument berilsa, VEB uchun qurilma ham qo'shiladi:
 * "Veb Android", "Veb iOS". Boshqa platformalarda qurilma ATAYLAB
 * e'tiborsiz qoldiriladi — mobil ilovada platformaning o'zi qurilma OS'i,
 * ya'ni "Android Android" bo'lib chiqardi. Bazadagi eskirgan `lastDevice`
 * (foydalanuvchi vebdan ilovaga o'tgan holat) ham shu tufayli zararsiz.
 */
export function platformLabel(p?: string | null, device?: string | null): string {
  const nomi = PLATFORM_LABELS[(p || "unknown") as ClientPlatform] ?? p ?? "Noma'lum";
  if (p !== "web") return nomi;
  const qoshimcha = DEVICE_SUFFIXES[(device || "") as ClientDevice];
  return qoshimcha ? `${nomi} ${qoshimcha}` : nomi;
}

export interface User {
  id: ID;
  telegramId?: number;
  phone: string;
  firstName: string;
  lastName: string;
  avatarUrl?: string;
  region?: string;
  district?: string;
  bio?: string;
  skills?: string[];
  completedJobsCount: number;
  isPhoneVerified: boolean;
  isBlocked: boolean;
  /**
   * Avtomatik moderatsiya bloki tugash vaqti (ISO). Bloklanmagan
   * foydalanuvchida umuman kelmaydi.
   *
   * Saqlashda `isBlocked` dan alohida — oqibati boshqacha: qo'lda qo'yilgan
   * blok ochilguncha turadi, bu esa muddati tugagach o'z-o'zidan kuchini
   * yo'qotadi. LEKIN panelda ikkalasi BITTA holat sifatida ko'rsatiladi
   * (`isUserBlocked`): admin uchun "bloklanganmi?" degan savolga ikkita
   * javob bo'lishi mumkin emas.
   */
  moderationBannedUntil?: string;
  /** Nega bloklangan — admin yozgan matn yoki avtomatik blok jumlasi. */
  blockReason?: string;
  /** Blokni kim qo'ygan. */
  blockSource?: "admin" | "moderation";
  /** Qachon bloklangan (ISO). */
  blockedAt?: string;
  /** Bloklagan admin id'si (faqat qo'lda blokda). */
  blockedBy?: string;
  /**
   * Ro'yxatdan o'tgan payt ishlatilgan klient. Bu funksiya qo'shilishidan
   * oldin ro'yxatdan o'tganlarda umuman kelmaydi — `platformLabel` uni
   * "Noma'lum" deb ko'rsatadi.
   */
  signupPlatform?: ClientPlatform;
  /** Oxirgi so'rov qaysi klientdan kelgan. */
  lastPlatform?: ClientPlatform;
  /**
   * Ro'yxatdan o'tgan paytdagi qurilma OS'i. FAQAT veb orqali kelganlarda
   * bo'ladi — mobil ilovada platformaning o'zi qurilma OS'i. Eski
   * hisoblarda umuman kelmaydi va bu normal: `platformLabel` unda
   * qo'shimchasiz "Veb" ko'rsatadi.
   */
  signupDevice?: ClientDevice;
  /** Oxirgi so'rov paytidagi qurilma OS'i. */
  lastDevice?: ClientDevice;
  /** `lastPlatform` qachon yozilgan (ISO). */
  lastSeenAt?: string;
  /** Hisob o'chirilganmi (foydalanuvchilardan olib tashlangan). */
  isDeleted?: boolean;
  /**
   * O'chirilgan hisobning raqami. O'chirishda `phone` bo'shatiladi (shu
   * raqam bilan qayta ro'yxatdan o'tish mumkin bo'lsin uchun) va qiymat
   * shu yerga ko'chadi.
   */
  deletedPhone?: string;
  /** Qachon o'chirilgani (ISO). */
  deletedAt?: string;
  /**
   * Hisob qachon yaratilgan — foydalanuvchi BIRINCHI marta ro'yxatdan
   * o'tgan payt (ISO). Backend uni `$setOnInsert` bilan yozadi, ya'ni
   * keyingi loginlar uni o'zgartirmaydi.
   *
   * Ixtiyoriy deb belgilangan, chunki bu tur ommaviy profil javobida ham
   * ishlatiladi — u yerda sana yuborilmaydi.
   */
  createdAt?: string;
  langPref?: "latin" | "cyrillic";
  themePref?: "light" | "dark";
  onboardingCompleted?: boolean;
}
/**
 * Moderatsiya bloki HOZIR kuchdami.
 *
 * Muddati o'tgan blok blok emas: backend ham shu qoidaga amal qiladi
 * (`RequireActiveUser` sanani solishtiradi), shuning uchun UI eskirgan
 * blokni "bloklangan" deb ko'rsatmasligi kerak.
 */
export function moderationBanUntil(u: Pick<User, "moderationBannedUntil">): Date | null {
  if (!u.moderationBannedUntil) return null;
  const d = new Date(u.moderationBannedUntil);
  return d.getTime() > Date.now() ? d : null;
}

/**
 * Foydalanuvchi bloklanganmi — manbasidan qat'i nazar.
 *
 * Panelda blok bitta tushuncha. Ikkita alohida belgi (qo'lda / avtomatik)
 * adminni chalg'itardi: "Faol" deb turgan foydalanuvchi aslida ilovaga kira
 * olmasligi mumkin edi.
 */
export function isUserBlocked(u: Pick<User, "isBlocked" | "moderationBannedUntil">): boolean {
  return !!u.isBlocked || !!moderationBanUntil(u);
}

/** Blok manbasi — inson o'qiy oladigan nom. */
export function blockSourceLabel(u: Pick<User, "isBlocked" | "moderationBannedUntil" | "blockSource">): string {
  if (moderationBanUntil(u)) return "Avtomatik (nomaqbul kontent)";
  if (u.isBlocked) return u.blockSource === "moderation" ? "Moderatsiya" : "Admin qarori";
  return "";
}

export interface Category {
  id: ID;
  name: string;
  slug: string;
  /** Public http(s) URL of the category icon (SVG or raster). */
  icon?: string;
  isSystemDefault?: boolean;
  isActive: boolean;
  /** Hozir feedda ko'rinib turgan (faol, vaqti o'tmagan) e'lonlar soni. */
  activeCount: number;
  /**
   * Tarixan joylangan e'lonlar soni. Ommaviy `/api/categories` javobida u
   * `activeCount` bilan bir xil qiymatga ega; tarixiy jami faqat admin
   * endpointidan keladi. Yangi kodda `activeCount` ishlatilsin.
   */
  usageCount: number;
  createdAt?: string;
}
// Ish e'loni kimlar uchun: erkaklar / ayollar / aralash.
export type Gender = "male" | "female" | "mixed";
export const GENDER_LABEL: Record<Gender, string> = {
  male: "Erkaklar",
  female: "Ayollar",
  mixed: "Aralash",
};
// Feed filtri va formalar uchun tartib (aralash — standart).
export const GENDER_OPTIONS: Gender[] = ["male", "female", "mixed"];

export interface Elon {
  id: ID;
  ownerId: ID;
  title: string;
  categoryId: ID;
  categoryName: string;
  description: string;
  locationUrl?: string;
  locationText?: string;
  lat?: number;
  lng?: number;
  region?: string;
  district?: string;
  workersNeeded: number;
  pricingType: "per_worker" | "total" | "negotiable";
  priceAmount: number;
  perWorkerAmount: number;
  startDate?: string;
  workTimeFrom?: string;
  workTimeTo?: string;
  contactPhone?: string;
  gender?: Gender;
  status: "draft" | "recruiting" | "filled" | "confirmed" | "in_progress" | "completed" | "cancelled" | "hidden";
  /**
   * Admin tomonidan foydalanuvchilardan olib tashlanganmi.
   *
   * `status: "hidden"` dan FARQLI: u qaytariladigan moderatsiya holati,
   * bu esa qaytarib bo'lmaydigan o'chirish. Ikkalasi bir vaqtda ham
   * bo'lishi mumkin.
   */
  isDeleted?: boolean;
  /** Qachon olib tashlangani (ISO). */
  deletedAt?: string;
  acceptedCount: number;
  viewsCount?: number;
  cancelledAt?: string;
  cancelReason?: string;
  updatedAt?: string;
  publishedAt?: string;
  createdAt: string;
  ownerName?: string;
  ownerAvatarUrl?: string;
  images?: string[];
}
export interface Application {
  id: ID;
  elonId: ID;
  elonTitle: string;
  workerId: ID;
  employerId: ID;
  workerPhone: string;
  workerName?: string;
  workerAvatarUrl?: string;
  workerRating?: number;
  workerReviewsCount?: number;
  ownerName?: string;
  ownerAvatarUrl?: string;
  peopleCount?: number;
  amount: number;
  isNegotiable: boolean;
  status: "pending" | "accepted" | "rejected" | "cancelled" | "completed";
  employerConfirmedDone?: boolean;
  workerConfirmedDone?: boolean;
  cancelledBy?: string;
  cancelReason?: string;
  appliedAt: string;
  decidedAt?: string;
  completedAt?: string;
}
export interface Notification {
  id: ID;
  type: string;
  title: string;
  body: string;
  isRead: boolean;
  createdAt: string;
  relatedEntity?: { type: string; id: ID };
}
export interface Feedback {
  id: ID;
  userId: ID;
  userName?: string;
  userPhone?: string;
  type: "suggestion" | "complaint";
  subject?: string;
  message: string;
  status: "open" | "resolved";
  createdAt: string;
}
// ----- admin types -----
export type AdminRole = "superadmin" | "moderator" | "support";

export interface Admin {
  id: ID;
  username: string;
  name?: string;
  role: AdminRole;
  isActive: boolean;
  totpEnabled: boolean;
  createdAt: string;
}

export interface AdminAudit {
  id: ID;
  adminId: ID;
  adminName?: string;
  action: string;
  target?: string;
  /**
   * Nishonning o'qiladigan nomi — SERVER hal qiladi (admin → `@login`,
   * turkum → nomi). Bo'sh bo'lsa nishon id bo'lib qoladi va audit sahifasi
   * uni `…a1b2c3` ko'rinishida qisqartiradi.
   */
  targetName?: string;
  detail?: string;
  createdAt: string;
}

/* ── Xatoliklar jurnali (Figma 3.12) ──────────────────────────────────
   Serverning yagona manbasi: `internal/errlog/catalog.go`. Qiymatlar shu
   yerda TAKRORLANADI (TypeScript backend turlarini ko'rmaydi), lekin
   ro'yxatlar YOPIQ: noma'lum daraja yoki modul kelsa, sahifa uni xom
   kod ko'rinishida ko'rsatadi — yashirmaydi. */

export type XatoDaraja = "critical" | "high" | "medium" | "low";

/**
 * Xatolik guruhining hayot sikli (Figma 3.12.3 · J), oltita holat.
 *
 * `regressed` — TIZIM belgisi: "Bartaraf etildi" deb yopilgan guruh qayta
 * takrorlanganda recorder qo'yadi (`internal/errlog/recorder.go`) va
 * darajani bir pog'ona ko'taradi. Uni panel qo'lda QO'YOLMAYDI —
 * `XatoQolHolat` ro'yxatida u yo'q va server ham 400 qaytaradi.
 */
export type XatoHolat =
  | "new"
  | "watching"
  | "fixing"
  | "resolved"
  | "regressed"
  | "ignored";

/** Admin O'ZI qo'ya oladigan holatlar (`errlog.ManualStatuses` bilan bir xil). */
export type XatoQolHolat = Exclude<XatoHolat, "regressed">;

export type XatoModul =
  | "backend"
  | "db"
  | "external"
  | "jobs"
  | "admin_app"
  | "client_app"
  | "security";

/**
 * Bitta xatolik GURUHI — bir xil `fingerprint` bo'yicha yig'ilgan hodisalar.
 *
 * Jurnalda foydalanuvchi ID'si, so'rov satri va telefon raqami YO'Q: server
 * ularni yozishdan oldin niqoblaydi (`internal/errlog/scrub.go`).
 * `usersCount` — noyob hash'lar soni, ya'ni "qancha odamga tegdi" degan
 * javob; "kim" degan javob saqlanmaydi.
 */
export interface AdminErrorGroup {
  id: ID;
  fingerprint: string;
  /** Panelda ko'rinadigan qisqa yorliq: `ERR-2F91C4`. */
  ref: string;
  code: string;
  module: XatoModul;
  severity: XatoDaraja;
  /** Qaysi muhitda yuz bergani: "Backend", "Admin ilova", "OTP bot", … */
  runtime: string;
  title: string;
  where?: string;
  message?: string;
  path?: string;
  /**
   * Oxirgi ma'lum qurilma va ilova versiyasi — ro'yxatdagi ikkita ustun
   * (Figma 3.12.3 · N). Mijoz `X-Client-Device` yubormasa bo'sh keladi va
   * panel "aniqlanmagan" ko'rsatadi.
   */
  lastDevice?: string;
  lastAppVersion?: string;
  count: number;
  usersCount: number;
  status: XatoHolat;
  note?: string;
  firstSeenAt: string;
  lastSeenAt: string;
  resolvedAt?: string;
  resolvedBy?: string;

  /* ── Hayot sikli (Figma 3.12.3 · J) ──────────────────────────────── */
  /** Katalogdagi ASL daraja — regressiya `severity`ni ko'targan bo'lsa. */
  baseSeverity?: XatoDaraja;
  /** Mas'ul adminning ko'rinadigan yorlig'i: "Aziz Karimov · superadmin". */
  assignee?: string;
  startedAt?: string;
  plannedVersion?: string;
  fixNote?: string;
  fixedVersion?: string;
  closedVersion?: string;
  reopenedAt?: string;
  /** "E'tiborsiz qoldirish" uchun MAJBURIY sabab (faqat superadmin yozadi). */
  ignoreReason?: string;
  activity?: XatoAmal[];
  /** Admin oxirgi marta qo'lda Telegram'ga yuborgan payt (60 s sovish oynasi). */
  tgSentAt?: string;
  /** Oxirgi AI tahlili — `POST /api/admin/errors/{id}/ai` saqlab qo'yadi. */
  ai?: XatoAI;
}

/**
 * AI ildiz-sabab xulosasi (Figma 3.12.1 · "Sababini aniqla").
 *
 * Matn SERVERDA yig'iladi (eksport bilan bir xil niqoblangan kontekst) va
 * Gemini'ga yuboriladi; natija guruhga yoziladi. Ya'ni bu yerdagi maydonlar
 * MODEL yozgan matn — ular fakt emas, taxmin. Panel shuning uchun `ishonch`
 * darajasini va model nomini har doim yonida ko'rsatadi.
 */
export interface XatoAI {
  /** Bir qatorli tashxis. */
  sarlavha: string;
  /** Ildiz sabab, 2–4 gap. */
  sabab: string;
  /** Eng aniq `fayl:qator` yoki komponent nomi; topilmasa "aniqlanmagan". */
  qayerda?: string;
  tuzatish?: string[];
  tekshirish?: string[];
  /** "past" | "o'rta" | "yuqori" — panel shu bo'yicha rang tanlaydi. */
  ishonch?: string;
  model?: string;
  /** Sarflangan token (hisobot uchun). */
  tokens?: number;
  include?: XatoKontekstKalit[];
  /**
   * Tahlil paytidagi hodisalar soni. `group.count` undan katta bo'lsa,
   * xulosa ESKIRGAN: xatolik o'zgargan sharoitda takrorlangan bo'lishi
   * mumkin va panel buni ochiq aytadi.
   */
  countAt?: number;
  at: string;
  by?: string;
}

/** "Amallar tarixi va izohlar" tasmasidagi bitta yozuv (oxirgi 50 ta). */
export interface XatoAmal {
  kind: "status" | "note" | "assign" | "telegram" | "regressed" | "export" | "ai";
  text: string;
  /** Bo'sh bo'lsa — amalni TIZIM bajargan (regressiya, avtoogohlantirish). */
  actor?: string;
  at: string;
}

/**
 * Hodisa yuz bergan qurilma (Figma 3.12.3 · H).
 *
 * Manba — `X-Client-Device` sarlavhasi yoki veb uchun `User-Agent`. HAR BIR
 * maydon bo'sh bo'lishi mumkin: mobil ilova hozircha bu sarlavhani
 * yubormaydi, shuning uchun panel bo'sh maydonni "aniqlanmagan" deb
 * ko'rsatadi — bo'sh katak qoldirmaydi.
 */
export interface XatoQurilma {
  platform?: string;
  brand?: string;
  model?: string;
  modelCode?: string;
  os?: string;
  osVersion?: string;
  apiLevel?: string;
  appVersion?: string;
  build?: string;
  flutter?: string;
  dart?: string;
  screen?: string;
  ram?: string;
  storage?: string;
  locale?: string;
  network?: string;
  battery?: string;
  emulator?: string;
  orientation?: string;
  browser?: string;
  engine?: string;
}

/** Xatolikdan OLDINGI qadam (breadcrumb, Figma 3.12.3 · I). */
export interface XatoQadam {
  at: string;
  kind: "nav" | "screen" | "action" | "request" | "response" | "crash";
  text: string;
}

/** Grafikning bitta soatlik ustuni. */
export interface XatoUstun {
  at: string;
  n: number;
}

/** Ta'sir taqsimotining bitta ulushi (Figma 3.12.3 · K). */
export interface XatoUlush {
  key: string;
  n: number;
  pct: number;
  /**
   * Ro'yxatga sig'magan mayda qiymatlarning QOLDIQ qatori — backend uni
   * o'zi qo'shadi ("Boshqa").
   *
   * NEGA alohida bayroq, `key` ni solishtirish emas: qoldiq qatori bitta
   * qiymat EMAS, bir necha qiymatning yig'indisi, ya'ni uni versiya yoki
   * brend qatori kabi o'qib bo'lmaydi. Yorliq matni esa o'zgaruvchan
   * (tarjima, qayta nomlash) — matn bo'yicha tekshiruv o'sha kuni
   * JIMGINA buzilib, qoldiq oddiy qiymat bo'lib ko'rina boshlardi.
   */
  other?: boolean;
}

export interface XatoTasir {
  brand: XatoUlush[];
  os: XatoUlush[];
  app: XatoUlush[];
}

/**
 * "So'nggi hodisalar" jadvalining bitta qatori.
 *
 * `user` — ism yoki `#A1B2C3` ko'rinishidagi hash yorlig'i. Telefon bu
 * yerda YO'Q: u faqat "Ta'sirlangan foydalanuvchilar" kartasida, u ham
 * niqoblangan holda.
 */
export interface XatoHodisa {
  at: string;
  user?: string;
  platform?: string;
  app?: string;
  network?: string;
  /** HTTP javob kodi satr sifatida ("500") yoki bo'sh. */
  status?: string;
  durationMs?: number;
  requestId?: string;
}

/** Eng boy namuna — stack trace, qurilma, so'rov va qadamlar shu yerdan. */
export interface XatoNamuna {
  at: string;
  device: XatoQurilma;
  deviceLabel?: string;
  message?: string;
  stack?: string[];
  steps?: XatoQadam[];
  method?: string;
  path?: string;
  status?: number;
  durationMs?: number;
  requestId?: string;
  /** Kim duch keldi (namunadagi id o'qish paytida nomga aylantiriladi). */
  actor?: string;
  actorRole?: string;
}

/**
 * "Ta'sirlangan foydalanuvchilar" qatori. `sub` — admin uchun rol, oddiy
 * foydalanuvchi uchun NIQOBLANGAN telefon ("+998 90 ••• •• 42").
 * Niqobni ochish imkoniyati panelda YO'Q — bunday endpoint yozilmagan.
 */
export interface XatoFoydalanuvchi {
  id?: ID;
  label: string;
  sub?: string;
  count: number;
  admin?: boolean;
}

/** `GET /api/admin/errors/{id}` — batafsil ekranning butun ma'lumoti. */
export interface AdminErrorDetail {
  group: AdminErrorGroup;
  /** Serverning o'z muhiti: `appEnv` va build SHA (`version`). */
  env: Record<string, string>;
  hourly: XatoUstun[];
  peak: XatoUstun;
  recent: XatoHodisa[];
  sample?: XatoNamuna;
  impact: XatoTasir;
  users: XatoFoydalanuvchi[];
  /** Guruhda hozir nechta to'liq namuna saqlanib turibdi (≤ 20). */
  samplesTotal: number;
  /**
   * `group.startedAt` dan KEYIN kelgan hodisalar soni (Figma 3.12.3 · J —
   * "Boshlanganidan beri 11 ta yangi hodisa").
   *
   * `group.count` o'rniga alohida son kerak: umumiy sanoq tuzatish
   * boshlangunga qadar to'plangan hodisalarni ham o'z ichiga oladi va
   * "ish qanday ketyapti" degan savolga javob bermaydi. `startedAt`
   * bo'lmasa maydon umuman kelmaydi.
   */
  sinceStarted?: number;
}

/** AI konteksti nimalardan yig'ilishi (Figma 3.12.3 · L · "Nimalar qo'shilsin"). */
export type XatoKontekstKalit =
  | "stack"
  | "device"
  | "request"
  | "steps"
  | "code"
  | "serverlog"
  | "similar";

/**
 * `GET /api/admin/errors/{id}/context` javobi.
 *
 * Matn SERVERDA yig'iladi va niqoblanadi. `masked` har doim `true`:
 * niqoblashni o'chiradigan parametr API'da UMUMAN YO'Q, shuning uchun
 * panelda ham o'chirib bo'lmaydigan tugma sifatida ko'rsatiladi.
 */
export interface XatoKontekst {
  format: "md" | "json" | "txt";
  text: string;
  chars: number;
  /** Taxminiy token soni (belgi / 4) — AI oynasiga sig'ishini baholash uchun. */
  tokens: number;
  masked: true;
  include: XatoKontekstKalit[];
  /** So'ralgan, lekin hozir MAVJUD BO'LMAGAN bo'laklar (server log, o'xshashlar). */
  unavailable: XatoKontekstKalit[];
  filename: string;
}

/** `GET /api/admin/errors/assignees` — mas'ul tanlash uchun tor ro'yxat. */
export interface XatoMasul {
  id: ID;
  label: string;
  role: AdminRole;
}

/**
 * `GET /api/admin/errors` javobi.
 *
 * `events` — FILTRGA MOS guruhlardagi hodisalar yig'indisi ("24 guruh ·
 * 1 284 hodisa"). `total` guruhlarni sanaydi, `events` — takrorlanishlarni;
 * ikkisi boshqa savolga javob beradi, shuning uchun ikkisi ham kerak.
 */
export interface PagedErrors extends Paged<AdminErrorGroup> {
  events: number;
}

/**
 * Beshta ko'rsatkich — JORIY FILTRDAN MUSTAQIL: ular butun tizimning
 * holati, tanlangan kesim emas (shu sababli alohida endpoint).
 */
export interface AdminErrorStats {
  open: number;
  critical: number;
  events24h: number;
  users24h: number;
  resolved7d: number;
  /** Server javob bergan payt (unix soniya) — "yangilandi" yozuvi uchun. */
  generatedAt: number;
}

export interface Broadcast {
  id: ID;
  title: string;
  body: string;
  region?: string;
  activeOnly: boolean;
  sentCount: number;
  status: "scheduled" | "sending" | "done";
  scheduledAt?: string;
  createdAt: string;
}

/**
 * Tarqatma segmenti — bazada haqiqatan uchraydigan bitta viloyat qiymati
 * va u topadigan qabul qiluvchilar soni (Figma 3.8b).
 *
 * Ro'yxat KODDA saqlanmaydi: `users.region` — tekshirilmaydigan erkin
 * matn, shuning uchun faqat server bazada nima borligini biladi.
 */
export interface BroadcastRegion {
  region: string;
  count: number;
}

export interface BroadcastRegions {
  items: BroadcastRegion[];
  /** «Barcha viloyatlar» segmentidagi odamlar soni. */
  total: number;
  /** Sonlar qaysi filtr bilan hisoblangani (formadagi katakcha). */
  activeOnly: boolean;
}

// Paged is the shape every admin list endpoint returns.
export interface Paged<T> {
  items: T[];
  page: number;
  limit: number;
  total: number;
}

/**
 * Admin arizalar jadvalining bitta qatori (Figma 3.6).
 *
 * `Application` dan ATAYLAB alohida tur: backend bu ro'yxatda faqat
 * jadvalda chizilgan maydonlarni qaytaradi (`appRowProjection`).
 * `Application` ni ishlatsak, TypeScript `employerId`, `workerId`,
 * `cancelReason` bor deb o'ylardi — kodda ular `undefined` bo'lib
 * chiqardi va bu xato faqat ish vaqtida bilinardi.
 */
export interface ApplicationRow {
  id: ID;
  elonId: ID;
  elonTitle: string;
  categoryName: string;
  workerName: string;
  workerPhone: string;
  amount: number;
  isNegotiable: boolean;
  status: string;
  appliedAt: string;
}

/**
 * `GET /api/admin/applications` javobi.
 *
 * `counts` — beshta voronka kartasi uchun holat bo'yicha sanoq, `overall`
 * — sarlavhadagi «Jami N ta ariza». Ikkalasi ham JORIY FILTRGA BOG'LIQ
 * EMAS: kartalar bir vaqtda ham taqsimotni ko'rsatadi, ham filtr tugmasi
 * bo'lib turadi (Figma 3.6a · 2-panel).
 */
export interface PagedApplications extends Paged<ApplicationRow> {
  counts: Record<string, number>;
  overall: number;
}

export interface DashboardStats {
  users: number;
  activeUsers: number;
  blockedUsers: number;
  todayUsers: number;
  elons: number;
  recruitingElons: number;
  filledElons: number;
  todayElons: number;
  completed: number;
  openReports: number;
  openFeedback: number;
  /**
   * Platforma bo'yicha foydalanuvchilar — OXIRGI ISHLATILGAN klient
   * bo'yicha ("hozir nimadan foydalanadi"). Ro'yxatdan o'tish taqsimoti
   * `AdminStats.platforms.signup` da.
   */
  webUsers: number;
  androidUsers: number;
  iosUsers: number;
  unknownPlatformUsers: number;
}

export interface DayPoint { date: string; count: number; }
export interface NameCount { name: string; count: number; }
/**
 * Platforma taqsimoti ikki kesimda. Ikkalasi kerak, chunki boshqa savolga
 * javob beradi: `signup` — o'sish qaysi kanaldan kelayotgani, `active` —
 * qaysi klientni rivojlantirish kerakligi. Vebdan ro'yxatdan o'tib ilovaga
 * ko'chgan oqim faqat shu ikkisi solishtirilganda ko'rinadi.
 */
export interface PlatformStats {
  signup: NameCount[];
  active: NameCount[];
  /** `active` necha kunlik oyna bo'yicha hisoblangani. */
  activeWindowDays: number;
}

export interface AdminStats {
  userGrowth: DayPoint[];
  elonGrowth: DayPoint[];
  funnel: Record<string, number>;
  topCategories: NameCount[];
  regions: NameCount[];
  platforms: PlatformStats;
}
