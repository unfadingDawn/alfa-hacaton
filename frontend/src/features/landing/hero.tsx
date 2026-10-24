"use client";

import { useEffect, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { BrandMark } from "@/components/brand";

const TAGLINES = [
  "Что произошло в экономике — и что это значит именно для вашей компании.",
  "Одна новость — разные выводы для финансового директора и руководителя закупок.",
  "Позиция аналитиков Альфа-Банка рядом с каждым важным событием.",
  "Риски и возможности для вашего бизнеса — со ссылкой на источник.",
];

// Подзаголовок, который сменяется справа налево. При «уменьшении движения» —
// без сдвига, только плавная смена.
export function RotatingTagline() {
  const [i, setI] = useState(0);
  const reduce = useReducedMotion();
  useEffect(() => {
    const t = setInterval(() => setI((x) => (x + 1) % TAGLINES.length), 4200);
    return () => clearInterval(t);
  }, []);
  return (
    <div className="mt-6 max-w-[560px]">
    <p className="sr-only">{TAGLINES.join(" ")}</p>
    <div className="relative h-[124px] overflow-hidden sm:h-[110px]" aria-hidden>
      <AnimatePresence mode="wait" initial={false}>
        <motion.p
          key={i}
          className="absolute inset-x-0 top-0 text-[22px] leading-snug text-ink sm:text-[26px]"
          initial={{ opacity: 0, x: reduce ? 0 : 64 }}
          animate={{ opacity: 1, x: 0 }}
          exit={{ opacity: 0, x: reduce ? 0 : -64 }}
          transition={{ duration: 0.55, ease: [0.22, 1, 0.36, 1] }}
        >
          {TAGLINES[i]}
        </motion.p>
      </AnimatePresence>
    </div>
      <div className="mt-3 flex gap-1.5" aria-hidden>
        {TAGLINES.map((_, j) => (
          <span key={j} className={j === i ? "h-1 w-6 rounded-full bg-steel transition-all" : "h-1 w-2 rounded-full bg-line transition-all"} />
        ))}
      </div>
    </div>
  );
}

// Крупный знак с переливающимся бликом и мягким стеклянным ореолом.
export function HeroMark() {
  const reduce = useReducedMotion();
  return (
    <div className="relative mx-auto flex aspect-square w-full max-w-[460px] items-center justify-center" aria-hidden>
      <div className="absolute inset-[8%] rounded-full bg-[radial-gradient(closest-side,rgb(239_49_36/0.22),transparent)] blur-2xl" />
      <div className="glass absolute inset-[14%] rounded-[44px] opacity-70" />
      <motion.div
        className="relative w-[78%]"
        animate={reduce ? undefined : { y: [0, -10, 0] }}
        transition={{ duration: 6, repeat: Infinity, ease: "easeInOut" }}
      >
        <BrandMark size={520} shimmer className="h-auto w-full drop-shadow-[0_24px_40px_rgb(239_49_36/0.28)]" />
      </motion.div>
    </div>
  );
}
