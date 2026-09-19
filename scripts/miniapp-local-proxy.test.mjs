import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { once } from "node:events";
import { createMiniAppProxy } from "./miniapp-local-proxy.mjs";

test("tunnel only forwards Mini App routes and never exposes admin or OTP", async () => {
  let calls = 0;
  const backend = http.createServer((_req, res) => { calls++; res.end("ok"); });
  backend.listen(0, "127.0.0.1");
  await once(backend, "listening");
  const proxy = createMiniAppProxy(backend.address().port);
  proxy.listen(0, "127.0.0.1");
  await once(proxy, "listening");
  try {
    const base = `http://127.0.0.1:${proxy.address().port}`;
    for (const path of ["/admin", "/api/admin/login", "/api/auth/otp/peek?token=secret", "/api/miniapp/auth/otp/peek", "/uploads/file.jpg", "/_next/image?url=http://127.0.0.1:8080/admin", "/miniapp/%2e%2e/admin"]) {
      assert.equal((await fetch(base + path)).status, 404);
    }
    assert.equal(calls, 0);
    assert.equal((await fetch(base + "/miniapp/post")).status, 200);
    assert.equal((await fetch(base + "/leaflet/leaflet.js")).status, 200);
    assert.equal((await fetch(base + "/api/miniapp/auth/miniapp/session", { method: "POST", body: "{}" })).status, 200);
    assert.equal(calls, 3);
  } finally { proxy.closeAllConnections(); proxy.close(); backend.closeAllConnections(); backend.close(); }
});
