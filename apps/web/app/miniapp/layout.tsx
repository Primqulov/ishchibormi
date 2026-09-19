import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "E'lon berish — Telegram",
  robots: { index: false, follow: false },
};

export default function MiniAppLayout({ children }: { children: React.ReactNode }) {
  return <div className="min-h-dvh bg-[color:var(--bg-subtle)]" style={{ paddingTop: "var(--tg-content-safe-area-inset-top, 0px)", paddingBottom: "max(env(safe-area-inset-bottom), var(--tg-content-safe-area-inset-bottom, 0px))" }}>{children}</div>;
}
