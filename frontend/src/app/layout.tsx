import type { Metadata, Viewport } from "next";
import { Toaster } from "sonner";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Альфа Инсайт", template: "%s — Альфа Инсайт" },
  description: "Персональная новостная аналитика для руководителей бизнеса: что произошло и как это влияет на вашу компанию.",
};

export const viewport: Viewport = { themeColor: "#f2f3f5" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body className="min-h-dvh font-sans">
        {children}
        <Toaster
          position="bottom-right"
          toastOptions={{
            classNames: {
              toast: "!rounded-[14px] !border-white/80 !bg-white/95 !text-ink !shadow-[var(--shadow-soft)] !font-sans",
              description: "!text-ink-soft",
            },
          }}
        />
      </body>
    </html>
  );
}
