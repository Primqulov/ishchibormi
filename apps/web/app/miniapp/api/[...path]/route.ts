import { NextRequest, NextResponse } from "next/server";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const backend = (process.env.MINIAPP_API_BASE_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const routes: Record<string, string[]> = {
  "auth/miniapp/session": ["POST"],
  categories: ["GET"],
  me: ["GET", "PATCH"],
  elons: ["POST"],
  uploads: ["POST", "DELETE"],
  image: ["GET"],
};

function failure(status: number, message: string) {
  return NextResponse.json({ error: { code: "miniapp_request_failed", message } }, { status, headers: { "Cache-Control": "no-store" } });
}

async function boundedBody(request: NextRequest, limit: number) {
  if (Number(request.headers.get("content-length") || 0) > limit) throw new Error("body_limit");
  const reader = request.body?.getReader();
  if (!reader) return undefined;
  const chunks: Uint8Array[] = [];
  let length = 0;
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    length += value.byteLength;
    if (length > limit) { await reader.cancel(); throw new Error("body_limit"); }
    chunks.push(value);
  }
  return Buffer.concat(chunks);
}

async function proxy(request: NextRequest, context: { params: { path: string[] } }) {
  const path = context.params.path.join("/");
  if (!routes[path]?.includes(request.method)) return failure(404, "Sahifa topilmadi.");
  let target = new URL(`${backend}/api/${path}`);
  target.search = request.nextUrl.search;
  if (path === "image") {
    // Local uploads cannot be fetched by a phone using localhost. Proxy only
    // the configured API's public listing images, never arbitrary URLs.
    try {
      const image = new URL(request.nextUrl.searchParams.get("url") || "");
      const api = new URL(backend);
      const localAlias = ["localhost", "127.0.0.1"].includes(image.hostname) && ["localhost", "127.0.0.1"].includes(api.hostname);
      if (image.protocol !== api.protocol || (image.origin !== api.origin && !(localAlias && image.port === api.port)) || image.username || image.password || image.search || image.hash || !/^\/uploads\/elons\/[a-zA-Z0-9_/-]+\.(jpg|jpeg|png|webp)$/.test(image.pathname)) return failure(404, "Rasm topilmadi.");
      target = new URL(image.pathname, api);
    } catch { return failure(404, "Rasm topilmadi."); }
  }
  // Mini App Telegram ichida ochiladi, oddiy saytda emas: admin panelida
  // bu foydalanuvchilar "Telegram" ustunida ko'rinishi kerak. Qurilma OS'i
  // baribir aniqlanadi (UA freymda haqiqiy brauzerniki) -> "Telegram iOS".
  const headers = new Headers({ "X-Client-Platform": "telegram" });
  for (const name of ["authorization", "content-type", "idempotency-key", "user-agent"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  try {
    const body = request.method === "GET" ? undefined : await boundedBody(request, path === "uploads" ? 9 << 20 : 64 << 10);
    const response = await fetch(target, { method: request.method, headers, body, cache: "no-store", redirect: "manual", signal: AbortSignal.timeout(60_000) });
    if (response.status >= 300 && response.status < 400) return failure(502, "Server javobini olishda xatolik yuz berdi.");
    return new NextResponse(response.body, { status: response.status, headers: {
      "Content-Type": response.headers.get("content-type") || "application/json",
      "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
    } });
  } catch (error) {
    return (error as Error).message === "body_limit" ? failure(413, "Yuborilgan fayl juda katta.") : failure(502, "Server vaqtincha ishlamayapti. Qayta urinib ko'ring.");
  }
}

export { proxy as GET, proxy as POST, proxy as PATCH, proxy as DELETE };
