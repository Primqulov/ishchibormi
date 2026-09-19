import http from "node:http";
import { fileURLToPath } from "node:url";

// Only the Mini App and its assets/API are exposed by the temporary tunnel.
// Admin, OTP/debug endpoints, the rest of the website and raw uploads stay local.
export function allowedMiniAppRequest(method, rawURL) {
  let path;
  try { path = decodeURIComponent(new URL(rawURL, "http://localhost").pathname); } catch { return false; }
  if (path.includes("\\") || path.split("/").some((part) => part === "." || part === "..")) return false;
  if (method === "GET" || method === "HEAD") {
    if (path === "/miniapp/post" || path === "/miniapp/post/" || path === "/favicon.ico") return true;
    if (/^\/(?:_next\/static|leaflet|fonts|img)\//.test(path)) return true;
    if (/^\/miniapp\/api\/(?:categories|me|image)$/.test(path)) return true;
  }
  if (method === "POST" && /^\/miniapp\/api\/(?:auth\/miniapp\/session|elons|uploads)$/.test(path)) return true;
  if (method === "PATCH" && path === "/miniapp/api/me") return true;
  if (method === "DELETE" && path === "/miniapp/api/uploads") return true;
  return false;
}

export function createMiniAppProxy(port = 3000) {
  return http.createServer((request, response) => {
    if (!allowedMiniAppRequest(request.method, request.url)) {
      response.writeHead(404, { "content-type": "text/plain", "cache-control": "no-store" });
      response.end("Not found");
      return;
    }
    const headers = { ...request.headers, host: `127.0.0.1:${port}` };
    for (const name of ["forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "connection", "upgrade"]) delete headers[name];
    const upstream = http.request({ hostname: "127.0.0.1", port, path: request.url, method: request.method, headers, timeout: 65_000 }, (result) => {
      response.writeHead(result.statusCode || 502, result.headers);
      result.pipe(response);
    });
    upstream.on("timeout", () => upstream.destroy());
    upstream.on("error", () => { if (!response.headersSent) response.writeHead(502); response.end("Mini App unavailable"); });
    request.on("aborted", () => upstream.destroy());
    request.pipe(upstream);
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const server = createMiniAppProxy();
  server.requestTimeout = 70_000;
  server.listen(3108, "127.0.0.1", () => process.stdout.write("Mini App proxy ready on 127.0.0.1:3108\n"));
}
