/** @type {import('next').NextConfig} */

// Statik ikon/brend assetlari uchun cache — Google favicon uchun "barqaror URL"
// tavsiya qiladi. 7 kun cache + stale-while-revalidate: brauzer/CDN saqlaydi,
// lekin logo yangilansa bir hafta ichida tarqaladi (immutable EMAS — shu bois
// keyingi o'zgarish qotib qolmaydi).
const ICON_CACHE = "public, max-age=604800, stale-while-revalidate=86400";

function originOf(raw, fallback) {
  try { return new URL(raw || fallback).origin; } catch { return fallback; }
}

const apiOrigin = originOf(process.env.NEXT_PUBLIC_API_BASE, "http://localhost:8080");
const devEval = process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : "";
const csp = [
  "default-src 'self'",
  "base-uri 'self'",
  "object-src 'none'",
  "frame-ancestors 'none'",
  "form-action 'self'",
  `script-src 'self' 'unsafe-inline'${devEval}`,
  "style-src 'self' 'unsafe-inline'",
  "font-src 'self' data:",
  `img-src 'self' data: blob: https: ${apiOrigin}`,
  // nominatim — MapPicker'da tanlangan nuqtaning manzil matnini olish uchun
  // (teskari geokodlash). Ro'yxatga qo'shilmasa fetch bloklanadi va xaritada
  // joy tanlaganda manzil yozuvi umuman chiqmaydi.
  `connect-src 'self' ${apiOrigin} https://nominatim.openstreetmap.org`,
  "worker-src 'self' blob:",
  "manifest-src 'self'",
].join("; ");

const SECURITY_HEADERS = [
  { key: 'Content-Security-Policy', value: csp },
  { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
  { key: 'X-Content-Type-Options', value: 'nosniff' },
  { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=(self), payment=(), usb=()' },
  { key: 'Cross-Origin-Opener-Policy', value: 'same-origin' },
  ...(process.env.NODE_ENV === 'production'
    ? [{ key: 'Strict-Transport-Security', value: 'max-age=63072000; includeSubDomains; preload' }]
    : []),
];

const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  // `next dev` va `next build` bir vaqtda ishlaganda bitta `.next` papkasini
  // almashib yubormasin. Aks holda ochiq development sahifasining CSS/JS
  // fayllari 404 qaytarib, UI va tugmalar ishlamay qoladi.
  distDir: process.env.NODE_ENV === 'development' ? '.next-dev' : '.next',
  async headers() {
    // Barcha root-darajadagi ikon fayllari (.png/.ico) + /img/ OG rasmlari.
    return [
      { source: '/:path*', headers: SECURITY_HEADERS },
      { source: '/:path((?!miniapp).*)', headers: [{ key: 'X-Frame-Options', value: 'DENY' }] },
      { source: '/miniapp/:path*', headers: [
        { key: 'Content-Security-Policy', value: csp.replace("frame-ancestors 'none'", "frame-ancestors https://web.telegram.org https://*.telegram.org").replace("script-src 'self'", "script-src 'self' https://telegram.org") },
        { key: 'X-Robots-Tag', value: 'noindex, nofollow' },
      ] },
      { source: '/:file([^/]+\\.ico)', headers: [{ key: 'Cache-Control', value: ICON_CACHE }] },
      { source: '/:file([^/]+\\.png)', headers: [{ key: 'Cache-Control', value: ICON_CACHE }] },
      { source: '/img/:path*', headers: [{ key: 'Cache-Control', value: ICON_CACHE }] },
    ];
  },
};
module.exports = nextConfig;
