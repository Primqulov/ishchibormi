// Butun sayt uchun HAQIQIY aloqa va ijtimoiy tarmoq ma'lumotlari — yagona manba (DRY).
// O'zgartirish kerak bo'lsa faqat shu yerni yangilang; barcha sahifalar shundan oladi.

export const CONTACT = {
  phone: "+998 90 020 25 35",
  phoneHref: "tel:+998900202535",
  email: "ishchibormi@gmail.com",
  emailHref: "mailto:ishchibormi@gmail.com",
} as const;

export type SocialLink = { label: string; href: string };

// Ro'yxatdan o'tish (OTP) boti — foydalanuvchi Telegramga yo'naltiriladigan
// barcha joylar (login, landing, bildirishnomalar) shu yagona manbadan oladi.
// Zaxira qiymat — HAQIQIY produksiya boti; NEXT_PUBLIC_BOT_USERNAME orqali
// (masalan test botga) qayta yo'naltirish mumkin.
export const AUTH_BOT_USERNAME =
  process.env.NEXT_PUBLIC_BOT_USERNAME || "Ishchibormibot";

export const AUTH_BOT: SocialLink = {
  label: `@${AUTH_BOT_USERNAME}`,
  href: `https://t.me/${AUTH_BOT_USERNAME}`,
};

// ── Android ilova ────────────────────────────────────────────────────────────
// Play Store sahifasi. Paket nomi flutter-app/android/app/build.gradle.kts
// dagi applicationId bilan bir xil bo'lishi SHART — App Links tasdiqlanishi
// ham shu paketga bog'liq (ishchibormi.uz/.well-known/assetlinks.json).
//
// Havolada `pcampaignid=web_share` kabi ulashish parametrlari ATAYLAB yo'q:
// ular Play ilovasi havolani ulashganda qo'shadigan vaqtinchalik belgilar,
// saytdagi doimiy havolada ular kerak emas.
export const APP_PACKAGE = "uz.ishchibormi.app";

export const APP = {
  playStore: `https://play.google.com/store/apps/details?id=${APP_PACKAGE}`,
  // Ilovaning minSdk 24 = Android 7.0 (flutter-app build'ining birlashtirilgan
  // manifestidan olingan). Flutter SDK yangilanganda bu qiymat ham o'zgarishi
  // mumkin — o'zgarsa shu yerni yangilang.
  minAndroid: "Android 7.0+",
} as const;

export const SOCIAL = {
  // Rasmiy Telegram kanali
  telegram: { label: "@Ishchibormi", href: "https://t.me/Ishchibormi" },
  // Qo'llab-quvvatlash (Telegram)
  support: { label: "@Ishchi_bormi_support", href: "https://t.me/Ishchi_bormi_support" },
  instagram: { label: "@ishchi_bormi", href: "https://instagram.com/ishchi_bormi" },
  youtube: { label: "@Ishchi_bormi", href: "https://youtube.com/@Ishchi_bormi" },
} as const satisfies Record<string, SocialLink>;

// Google/schema.org "sameAs" uchun ochiq ijtimoiy tarmoq profillari.
export const SOCIAL_SAMEAS: string[] = [
  SOCIAL.telegram.href,
  SOCIAL.instagram.href,
  SOCIAL.youtube.href,
];
