"use client";
import { ReactNode, useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { getAccess } from "@/lib/api";
import { loginPath } from "@/lib/auth-redirect";

/**
 * CabinetLayout is the auth boundary for every page in the (cabinet) group.
 *
 * The guard is render-blocking on purpose: protected content is NOT painted
 * until a token is confirmed present. An effect-only check (like the one in
 * <Shell/>) still renders the cabinet for one frame and — critically — is
 * skipped entirely when the browser restores the page from bfcache (pressing
 * "Back" after logout), which let a logged-out user keep using the cabinet.
 *
 * We re-run the check on navigation and on pageshow/focus/storage so that
 * logging out (here or in another tab) or hitting Back immediately bounces the
 * user to /login instead of leaving a stale, still-usable cabinet on screen.
 */
export default function CabinetLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  // Starts false so the first paint (and SSR output, where localStorage is
  // unavailable) shows nothing until the client confirms a token exists —
  // this also keeps client/server markup identical, avoiding a hydration warp.
  const [authed, setAuthed] = useState(false);

  useEffect(() => {
    const check = () => {
      if (getAccess()) {
        setAuthed(true);
      } else {
        setAuthed(false);
        router.replace(loginPath());
      }
    };
    check();

    // bfcache restore (Back after logout) doesn't re-run effects, but it does
    // fire `pageshow`. focus/visibility/storage cover logout in another tab.
    const onShow = () => check();
    const onVisible = () => {
      if (document.visibilityState === "visible") check();
    };
    window.addEventListener("pageshow", onShow);
    window.addEventListener("focus", onShow);
    window.addEventListener("storage", onShow);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.removeEventListener("pageshow", onShow);
      window.removeEventListener("focus", onShow);
      window.removeEventListener("storage", onShow);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [router, pathname]);

  // Not authenticated → render nothing while the redirect to /login runs.
  if (!authed) return null;

  return <>{children}</>;
}
