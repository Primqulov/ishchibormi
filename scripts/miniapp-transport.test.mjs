import { test } from "node:test";
import assert from "node:assert/strict";
import { fetchMiniApp } from "../apps/web/lib/miniapp-transport.ts";

const post = {
  method: "POST",
  headers: { "Idempotency-Key": "0123456789abcdef01234567", "Content-Type": "application/json" },
  body: '{"title":"Hovlini tozalash"}',
};

test("a disconnected tunnel recovers and preserves the listing's body and key", async () => {
  const requests = [], waits = [];
  const result = await fetchMiniApp("/miniapp/api/elons", post, {
    fetcher: async (url, init) => {
      requests.push({ url, body: init.body, key: new Headers(init.headers).get("idempotency-key") });
      if (requests.length === 1) return new Response("error code: 1033", { status: 530 });
      if (requests.length === 2) throw new TypeError("connection lost");
      return Response.json({ id: "same-listing" }, { status: 201 });
    },
    wait: async (ms) => { waits.push(ms); },
  });
  assert.equal(result.response.status, 201);
  assert.equal(JSON.parse(result.text).id, "same-listing");
  assert.equal(requests.length, 3);
  assert.deepEqual(requests, Array(3).fill(requests[0]));
  assert.deepEqual(waits, [1000, 2500]);
});

test("lost success responses can recover the same listing", async () => {
  const saved = new Map(); let calls = 0;
  const result = await fetchMiniApp("/miniapp/api/elons", post, {
    fetcher: async (_url, init) => {
      calls++;
      const key = new Headers(init.headers).get("idempotency-key");
      if (!saved.has(key)) saved.set(key, { id: "created-once" });
      if (calls === 1) return { status: 201, text: async () => { throw new TypeError("response interrupted"); } };
      return Response.json(saved.get(key));
    }, wait: async () => {},
  });
  assert.equal(saved.size, 1);
  assert.equal(calls, 2);
  assert.equal(JSON.parse(result.text).id, "created-once");
});

test("retries stop after four attempts and preserve the final response", async () => {
  let calls = 0;
  const result = await fetchMiniApp("/miniapp/api/categories", {}, {
    fetcher: async () => { calls++; return new Response("unavailable", { status: 503 }); }, wait: async () => {},
  });
  assert.equal(calls, 4);
  assert.equal(result.response.status, 503);
  assert.equal(result.text, "unavailable");
});

test("validation, authentication and moderation errors are not retried", async () => {
  for (const status of [400, 401, 403, 409, 422, 429, 500]) {
    let calls = 0;
    await fetchMiniApp("/miniapp/api/elons", post, {
      fetcher: async () => { calls++; return new Response("rejected", { status }); },
      wait: async () => assert.fail("must not retry"),
    });
    assert.equal(calls, 1);
  }
});

test("uploads, profile updates and unkeyed writes are never replayed", async () => {
  for (const [url, init] of [
    ["/miniapp/api/uploads", post], ["/miniapp/api/me", { ...post, method: "PATCH" }],
    ["/miniapp/api/elons", { ...post, headers: {} }], ["/api/elons", post],
  ]) {
    let calls = 0;
    await assert.rejects(fetchMiniApp(url, init, {
      fetcher: async () => { calls++; throw new TypeError("lost response"); },
      wait: async () => assert.fail("must not retry"),
    }));
    assert.equal(calls, 1);
  }
});

test("session initialization can recover and cancellation stops retrying", async () => {
  let calls = 0;
  await fetchMiniApp("/miniapp/api/auth/miniapp/session", { method: "POST", body: "{}" }, {
    fetcher: async () => { calls++; return Response.json({}, { status: calls === 1 ? 502 : 200 }); }, wait: async () => {},
  });
  assert.equal(calls, 2);
  const abort = new AbortController();
  calls = 0;
  await assert.rejects(fetchMiniApp("/miniapp/api/elons", { ...post, signal: abort.signal }, {
    fetcher: async () => { calls++; return new Response("offline", { status: 530 }); },
    wait: async () => { abort.abort(); },
  }));
  assert.equal(calls, 1);
});
