export interface TelegramWebApp {
  initData: string;
  initDataUnsafe?: { user?: { first_name?: string; last_name?: string } };
  colorScheme: "light" | "dark";
  ready(): void;
  expand(): void;
  close(): void;
  enableClosingConfirmation(): void;
  disableClosingConfirmation(): void;
  openTelegramLink(url: string): void;
  requestContact(callback: (sent: boolean, event?: { response?: string }) => void): void;
  onEvent(name: string, callback: () => void): void;
  offEvent(name: string, callback: () => void): void;
}

declare global {
  interface Window { Telegram?: { WebApp?: TelegramWebApp } }
}

export function isMiniAppPage() {
  return typeof window !== "undefined" && /^\/miniapp(?:\/|$)/.test(window.location.pathname);
}

export function uploadPreviewURL(url: string) {
  if (!isMiniAppPage()) return url;
  try {
    const parsed = new URL(url);
    if (parsed.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(parsed.hostname)) {
      return `/api/miniapp/image?url=${encodeURIComponent(url)}`;
    }
  } catch { /* Preserve the original value; upload validation runs server-side. */ }
  return url;
}
