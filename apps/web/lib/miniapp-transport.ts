const RETRY_DELAYS = [1000, 2500, 5000];

export function isTransientMiniAppStatus(status: number) {
  // Includes Cloudflare's disconnected-tunnel response (HTTP 530 / error 1033).
  return [502, 503, 504, 520, 521, 522, 523, 524, 530].includes(status);
}

function canRetry(url: string, init: RequestInit) {
  const path = new URL(url, "https://miniapp.invalid").pathname;
  if (!path.startsWith("/miniapp/api/")) return false;
  const method = (init.method || "GET").toUpperCase();
  if (method === "GET") return true;
  if (method !== "POST" || typeof init.body !== "string") return false;
  if (path === "/miniapp/api/auth/miniapp/session") return true;
  // The API deduplicates listing writes using this exact key and owner.
  // Uploads and other writes cannot safely be replayed after a lost response.
  return path === "/miniapp/api/elons" && /^[a-f\d]{24}$/i.test(new Headers(init.headers).get("Idempotency-Key") || "");
}

function delay(ms: number, signal?: AbortSignal | null) {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) { reject(signal.reason); return; }
    const onAbort = () => { clearTimeout(timer); reject(signal?.reason); };
    const timer = setTimeout(() => { signal?.removeEventListener("abort", onAbort); resolve(); }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

export async function fetchMiniApp(
  url: string,
  init: RequestInit,
  dependencies: { fetcher?: typeof fetch; wait?: typeof delay } = {},
): Promise<{ response: Response; text: string }> {
  const fetcher = dependencies.fetcher || fetch;
  const wait = dependencies.wait || delay;
  const retryable = canRetry(url, init);
  for (let attempt = 0; ; attempt++) {
    if (init.signal?.aborted) throw init.signal.reason;
    try {
      const response = await fetcher(url, init);
      const text = await response.text();
      if (!retryable || !isTransientMiniAppStatus(response.status) || attempt === RETRY_DELAYS.length) {
        return { response, text };
      }
    } catch (error) {
      if (!retryable || init.signal?.aborted || attempt === RETRY_DELAYS.length) throw error;
    }
    await wait(RETRY_DELAYS[attempt], init.signal);
  }
}
