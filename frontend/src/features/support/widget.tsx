"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Maximize2, MessageCircleQuestion, X } from "lucide-react";
import { SupportChat } from "./chat";

const TEASER_KEY = "alfainsight:support-teaser:v1";
const TEASER_DELAY_MS = 5000;
const TEASER_VISIBLE_MS = 14000;

function readSeen() {
  try {
    return window.localStorage.getItem(TEASER_KEY) === "1";
  } catch {
    return true; // хранилище недоступно — не показываем подсказку повторно на каждой странице
  }
}

function markSeen() {
  try {
    window.localStorage.setItem(TEASER_KEY, "1");
  } catch {
    /* хранилище недоступно */
  }
}

// Плавающая кнопка поддержки в правом нижнем углу с ИИ-помощником.
// При первом визите через несколько секунд один раз показывается ненавязчивая
// подсказка; она сама скрывается и больше не появляется.
export function SupportWidget() {
  const path = usePathname();
  const reduce = useReducedMotion();
  const [open, setOpen] = useState(false);
  const [teaser, setTeaser] = useState(false);

  useEffect(() => {
    if (readSeen()) return;
    const show = setTimeout(() => {
      setTeaser(true);
      markSeen();
    }, TEASER_DELAY_MS);
    return () => clearTimeout(show);
  }, []);

  useEffect(() => {
    if (!teaser) return;
    const hide = setTimeout(() => setTeaser(false), TEASER_VISIBLE_MS);
    return () => clearTimeout(hide);
  }, [teaser]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (path.startsWith("/support")) return null;

  const toggle = () => {
    setTeaser(false);
    markSeen();
    setOpen((v) => !v);
  };

  return (
    <>
      <AnimatePresence>
        {teaser && !open && (
          <motion.div
            role="status"
            initial={reduce ? { opacity: 0 } : { opacity: 0, y: 10, scale: 0.96 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 6 }}
            transition={{ duration: 0.25 }}
            className="fixed bottom-[156px] right-4 z-40 w-[min(300px,calc(100vw-32px))] rounded-[16px] border border-line/70 bg-white/97 p-3.5 pr-9 backdrop-blur-xl shadow-[var(--shadow-soft)] lg:bottom-[92px] lg:right-6"
          >
            <button
              onClick={() => setTeaser(false)}
              className="absolute right-2 top-2 rounded-full p-1 text-silver hover:text-ink"
              aria-label="Скрыть подсказку"
            >
              <X className="size-4" />
            </button>
            <p className="text-[14px] font-medium">Есть вопросы о сервисе?</p>
            <p className="mt-0.5 text-[13px] leading-snug text-ink-soft">ИИ-помощник подскажет, как запустить анализ, сменить роль или проверить источники.</p>
            <button onClick={toggle} className="mt-2 text-[13px] font-medium text-steel hover:underline">
              Спросить помощника
            </button>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {open && (
          <motion.section
            role="dialog"
            aria-label="ИИ-помощник поддержки"
            initial={reduce ? { opacity: 0 } : { opacity: 0, y: 16, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduce ? { opacity: 0 } : { opacity: 0, y: 12, scale: 0.98 }}
            transition={{ duration: 0.22, ease: [0.2, 0.8, 0.2, 1] }}
            className="fixed inset-x-3 bottom-[156px] top-20 z-40 flex flex-col border border-line/70 bg-white/97 backdrop-blur-xl overflow-hidden rounded-[var(--radius-xl)] shadow-[0_24px_60px_-20px_rgb(0_0_0/0.35)] lg:inset-x-auto lg:bottom-[92px] lg:right-6 lg:top-auto lg:h-[min(620px,calc(100dvh-130px))] lg:w-[400px]"
          >
            <header className="flex items-center gap-3 border-b border-line/80 px-4 py-3">
              <span className="flex size-9 items-center justify-center rounded-full bg-steel/10 text-steel">
                <MessageCircleQuestion className="size-5" aria-hidden />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-[15px] font-medium">Помощник «Альфа Инсайт»</p>
                <p className="text-[12px] text-silver">Вопросы о работе сервиса</p>
              </div>
              <Link href="/support" onClick={() => setOpen(false)} className="rounded-full p-1.5 text-silver hover:text-ink" aria-label="Открыть на всю страницу">
                <Maximize2 className="size-4" />
              </Link>
              <button onClick={() => setOpen(false)} className="rounded-full p-1.5 text-silver hover:text-ink" aria-label="Закрыть помощника">
                <X className="size-5" />
              </button>
            </header>
            <SupportChat compact autoFocus />
          </motion.section>
        )}
      </AnimatePresence>

      <motion.button
        onClick={toggle}
        aria-label={open ? "Закрыть помощника" : "Поддержка: спросить ИИ-помощника"}
        aria-expanded={open}
        whileHover={reduce ? undefined : { scale: 1.06 }}
        whileTap={reduce ? undefined : { scale: 0.94 }}
        className="fixed bottom-[88px] right-4 z-40 flex size-14 items-center justify-center rounded-full bg-steel text-white shadow-[0_12px_28px_-10px_rgb(239_49_36/0.75)] lg:bottom-6 lg:right-6"
      >
        {teaser && !open && !reduce && (
          <span className="absolute inset-0 animate-ping rounded-full bg-steel/40" aria-hidden />
        )}
        <AnimatePresence mode="wait" initial={false}>
          <motion.span
            key={open ? "x" : "q"}
            initial={{ rotate: -90, opacity: 0 }}
            animate={{ rotate: 0, opacity: 1 }}
            exit={{ rotate: 90, opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="relative"
          >
            {open ? <X className="size-6" /> : <MessageCircleQuestion className="size-6" />}
          </motion.span>
        </AnimatePresence>
      </motion.button>
    </>
  );
}
