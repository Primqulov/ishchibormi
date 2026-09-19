/** A notification destination must stay on this site after login. */
export function safeReturnPath(raw: string | null | undefined): string {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//") || /[\\\u0000-\u001f\u007f]/.test(raw)) return "/dashboard";
  try {
    const url = new URL(raw, "https://return.local");
    if (url.origin !== "https://return.local" || ["/login", "/onboarding"].includes(url.pathname)) return "/dashboard";
    return url.pathname + url.search + url.hash;
  } catch {
    return "/dashboard";
  }
}

export function loginPath(): string {
  if (typeof window === "undefined") return "/login";
  const next = safeReturnPath(window.location.pathname + window.location.search + window.location.hash);
  return "/login?next=" + encodeURIComponent(next);
}

export function returnPath(): string {
  return safeReturnPath(new URLSearchParams(window.location.search).get("next"));
}
