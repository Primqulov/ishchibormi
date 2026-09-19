"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Briefcase, History, Settings, User as UserIcon, Plus, LogOut,
  Bell as BellIcon, MessageSquareWarning, Menu, Search, X, LayoutGrid,
  ListChecks, FileText, AlertTriangle, RefreshCw,
} from "lucide-react";
import { api, getAccess, Notification, setAccess, User } from "@/lib/api";
import { loginPath, safeReturnPath } from "@/lib/auth-redirect";
import { T, useT } from "@/components/T";
import { LangMenu } from "@/components/LangMenu";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Avatar } from "@/components/ui/Avatar";
import { Logo } from "@/components/Logo";

/** Figma "Nav/TopBar" — asosiy havolalar. */
const NAV: { href: string; label: string; icon: any }[] = [
  { href: "/dashboard", label: "Bosh sahifa",        icon: LayoutGrid },
  { href: "/elonlar",   label: "Ish e'lonlari",      icon: Briefcase },
  { href: "/process",   label: "Mening arizalarim",  icon: FileText },
  { href: "/my-elons",  label: "Mening e'lonlarim",  icon: ListChecks },
];

/** Avatar menyusidagi qo'shimcha bo'limlar. */
const MENU: { href: string; label: string; icon: any }[] = [
  { href: "/profile",  label: "Profil",                icon: UserIcon },
  { href: "/history",  label: "Ishlar tarixi",         icon: History },
  { href: "/settings", label: "Sozlamalar",            icon: Settings },
  { href: "/feedback", label: "Taklif va shikoyatlar", icon: MessageSquareWarning },
];

export function Shell({
  title,
  wide,
  children,
}: {
  title?: string;
  /** Sahifa o'z sarlavhasini o'zi chizsa (masalan bosh sahifa hero'si) — true. */
  wide?: boolean;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useT();
  const qc = useQueryClient();
  const [drawer, setDrawer] = useState(false);
  const [menu, setMenu] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Auth gate
  useEffect(() => { if (!getAccess()) router.replace(loginPath()); }, [router]);

  const {
    data: me,
    error: meQueryError,
    isError: meError,
    refetch: refetchMe,
  } = useQuery<User>({
    queryKey: ["me"],
    queryFn: () => api.get<User>("/api/me"),
    retry: false,
  });

  // Sessiya tugagan bo'lsa /api/me 401 (yoki hisob o'chirilgan bo'lsa 403
  // account_disabled) qaytaradi va api.ts tokenlarni tozalaydi. Token
  // tozalangan bo'lsa (getAccess() null) — foydalanuvchini login sahifasiga
  // yo'naltiramiz. Boshqa (masalan server) xatolarida bu ishlamaydi.
  //
  // qc.clear() shart: aks holda react-query keshidagi eski `me` saqlanib
  // qoladi va o'sha brauzerda yangi hisobga kirilganda profil bir zumga eski
  // ism-familiyani ko'rsatib yuboradi.
  useEffect(() => {
    if (meError && !getAccess()) {
      qc.clear();
      router.replace(loginPath());
    }
  }, [meError, router, qc]);

  const { data: notifs } = useQuery<Notification[]>({
    queryKey: ["notifications"],
    queryFn: () => api.get<Notification[]>("/api/notifications"),
    refetchInterval: 30000,
  });
  const unread = (notifs || []).filter((n) => !n.isRead).length;
  // "Mening arizalarim" bo'limi uchun nuqta: ariza bilan bog'liq (yangi ariza,
  // qabul/rad/bekor, yakunlash) o'qilmagan bildirishnoma bormi.
  const processDot = (notifs || []).some((n) => !n.isRead && n.relatedEntity?.type === "application");

  // onboarding redirect
  useEffect(() => {
    if (me && !me.onboardingCompleted && pathname && !pathname.startsWith("/onboarding")) {
      const next = safeReturnPath(window.location.pathname + window.location.search + window.location.hash);
      router.replace("/onboarding?next=" + encodeURIComponent(next));
    }
  }, [me, pathname, router]);

  // Avatar menyusi — tashqariga bosilganda yopiladi.
  useEffect(() => {
    if (!menu) return;
    function onDown(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenu(false);
    }
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [menu]);

  // Marshrut almashganda ochiq menyular yopilsin.
  useEffect(() => { setMenu(false); setDrawer(false); }, [pathname]);

  function logout() {
    setAccess(null);
    // Oldingi sessiyaning keshlangan ma'lumotlari (profil, bildirishnomalar,
    // ro'yxatlar) yangi/keyingi foydalanuvchida ko'rinib qolmasligi uchun.
    qc.clear();
    router.replace("/login");
  }

  const fullName = me ? `${me.firstName} ${me.lastName}` : "";
  const isActive = (href: string) => pathname === href || (pathname?.startsWith(href + "/") ?? false);
  const serviceError = meError && getAccess() ? userErrorMessage(meQueryError) : "";

  return (
    <div className="min-h-screen flex flex-col">
      {/* ── Top bar — Figma "Nav/TopBar": h 76, px 40, border-b ───────── */}
      <header
        className="sticky top-0 z-40 border-b backdrop-blur-md"
        style={{ borderColor: "var(--border)", background: "color-mix(in srgb, var(--card) 92%, transparent)" }}
      >
        <div className="mx-auto max-w-shell flex h-[68px] md:h-[76px] items-center gap-3 md:gap-4 px-4 md:px-10">
          <button
            onClick={() => setDrawer(true)}
            className="xl:hidden -ml-1 p-2 rounded-lg muted hover:bg-[color:var(--bg-subtle)] shrink-0"
            aria-label={t("Menyu")}
          >
            <Menu size={20} />
          </button>

          <Logo href="/dashboard" />

          {/* Havolalar faqat xl (≥1280px) da ko'rinadi — kichikroq ekranda ular
              menyuga tushadi. O'ng tomondagi tugmalar shrink-0, qisilishi mumkin
              bo'lgan yagona element shu nav: joy yetmasa avatarni chetga itarish
              o'rniga o'zi qisqaradi. */}
          <nav className="hidden xl:flex items-center gap-1 min-w-0 overflow-hidden">
            {NAV.map(({ href, label }) => (
              <Link key={href} href={href} className={`navlink shrink-0 ${isActive(href) ? "navlink-active" : ""}`}>
                <T>{label}</T>
                {href === "/process" && processDot && (
                  <span className="h-1.5 w-1.5 rounded-full" style={{ background: "var(--accent)" }} />
                )}
              </Link>
            ))}
          </nav>

          <div className="flex-1 min-w-0" />

          <Link href="/elon/create" className="btn btn-primary gap-1.5 shrink-0 !px-4 md:!px-5 !py-2.5">
            <Plus size={16} /><span className="hidden sm:inline"><T>E'lon berish</T></span>
          </Link>

          <div className="hidden sm:flex items-center gap-1 shrink-0">
            <LangMenu />
            <ThemeToggle />
          </div>

          <Link href="/notifications" className="icon-btn relative shrink-0" aria-label={t("Bildirishnomalar")}>
            <BellIcon size={19} />
            {unread > 0 && (
              <span
                className="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] px-1 grid place-items-center rounded-full text-white text-[10px] font-bold ring-2"
                style={{ background: "var(--accent)", ["--tw-ring-color" as any]: "var(--card)" }}
              >
                {unread > 9 ? "9+" : unread}
              </span>
            )}
          </Link>

          {/* Avatar + menyu */}
          <div className="relative shrink-0" ref={menuRef}>
            <button
              onClick={() => setMenu((s) => !s)}
              className="relative rounded-full transition hover:opacity-90"
              aria-label={t("Profil menyusi")}
            >
              <span className="grid h-11 w-11 place-items-center rounded-full overflow-hidden"
                    style={{ background: "var(--brand-100)" }}>
                {me?.avatarUrl
                  ? <Avatar name={fullName} src={me.avatarUrl} size="md" />
                  : <span className="font-bold text-[15px]" style={{ color: "var(--brand)" }}>
                      {(me?.firstName?.[0] || "?").toUpperCase()}
                    </span>}
              </span>
              <span className="absolute bottom-0.5 right-0.5 h-2.5 w-2.5 rounded-full ring-2"
                    style={{ background: "var(--accent)", ["--tw-ring-color" as any]: "var(--card)" }} />
            </button>

            {menu && (
              <div className="card-elevated absolute right-0 mt-2 w-60 p-2 animate-scale-in z-50">
                <Link href="/profile" className="flex items-center gap-3 rounded-xl p-2 hover:bg-[color:var(--bg-subtle)] transition">
                  <Avatar name={fullName} src={me?.avatarUrl} size="sm" />
                  <div className="min-w-0">
                    <div className="text-sm font-bold truncate heading">{fullName || "—"}</div>
                    <div className="text-xs subtle"><T>Profilni ko'rish</T></div>
                  </div>
                </Link>
                <div className="divider my-2" />
                {MENU.map(({ href, label, icon: Icon }) => (
                  <Link key={href} href={href} className={`sidenav-item ${isActive(href) ? "sidenav-item-active" : ""}`}>
                    <Icon size={17} /><T>{label}</T>
                  </Link>
                ))}
                <div className="divider my-2" />
                <button onClick={logout} className="sidenav-item w-full text-danger hover:bg-[color:var(--danger-bg,rgba(217,45,32,0.08))]">
                  <LogOut size={17} /><T>Chiqish</T>
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      {serviceError && (
        <div
          role="alert"
          className="border-b px-4 md:px-10 py-3"
          style={{
            borderColor: "var(--danger-border, #fecaca)",
            background: "var(--danger-bg, #fef2f2)",
          }}
        >
          <div className="mx-auto max-w-shell flex items-center gap-3 text-[13px]">
            <AlertTriangle size={18} className="text-danger shrink-0" />
            <span className="flex-1 heading">{serviceError}</span>
            <button
              type="button"
              className="btn btn-outline !py-2 !px-3 gap-1.5 shrink-0"
              onClick={() => void refetchMe()}
            >
              <RefreshCw size={14} />
              <T>Qayta urinish</T>
            </button>
          </div>
        </div>
      )}

      {/* ── Mobil menyu ─────────────────────────────────────────── */}
      {drawer && (
        <div className="fixed inset-0 z-50 xl:hidden">
          <div className="absolute inset-0 bg-black/40 animate-fade-in" onClick={() => setDrawer(false)} />
          <aside className="relative w-[290px] h-full p-4 flex flex-col gap-1 animate-slide-up"
                 style={{ background: "var(--card)" }}>
            <div className="flex items-center justify-between mb-3">
              <Logo href="/dashboard" />
              <button onClick={() => setDrawer(false)} className="p-2 rounded-lg muted hover:bg-[color:var(--bg-subtle)]">
                <X size={18} />
              </button>
            </div>
            {[...NAV, ...MENU].map(({ href, label, icon: Icon }) => (
              <Link key={href} href={href} className={`sidenav-item ${isActive(href) ? "sidenav-item-active" : ""}`}>
                <Icon size={17} /><span className="flex-1"><T>{label}</T></span>
                {href === "/process" && processDot && (
                  <span className="h-2 w-2 rounded-full" style={{ background: "var(--accent)" }} />
                )}
              </Link>
            ))}
            <div className="divider my-2" />
            <div className="flex items-center gap-2 px-1"><LangMenu /><ThemeToggle /></div>
            <div className="flex-1" />
            <button onClick={logout} className="sidenav-item text-danger">
              <LogOut size={17} /><T>Chiqish</T>
            </button>
          </aside>
        </div>
      )}

      {/* ── Sahifa mazmuni ─────────────────────────────────────── */}
      <main className={`flex-1 mx-auto w-full max-w-shell px-4 md:px-10 ${wide ? "py-0" : "py-6"} min-w-0 animate-fade-in`}>
        {!wide && title && (
          <div className="flex items-center justify-between gap-3 mb-5 flex-wrap">
            <h1 className="text-[26px] font-black heading tracking-[-0.5px] leading-tight"><T>{title}</T></h1>
          </div>
        )}
        <div className="flex flex-col gap-5 min-w-0">{children}</div>
      </main>
    </div>
  );
}

function userErrorMessage(error: unknown): string {
  if (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof error.message === "string"
  ) {
    return error.message;
  }
  return "Ma'lumotlarni yuklab bo'lmadi. Qayta urinib ko'ring.";
}

/* ── Sahifa qidiruvi — Figma "Search" pill. Navbarda emas, sahifa mazmunida
      ishlatiladi: topbarda joy yetmay, o'ng tomondagi tugmalar siqilib qolardi. ── */
export function ShellSearch({
  value,
  onChange,
  placeholder,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  /** Inputga qo'shimcha klasslar — masalan yonidagi tugma bilan bir xil balandlik. */
  className?: string;
}) {
  return (
    <div className="relative">
      <Search size={16} className="absolute left-4 top-1/2 -translate-y-1/2 subtle pointer-events-none" />
      <input
        className={`input !pl-11 ${className || ""}`}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}
