import Link from "next/link";
import { Building2, FileSearch, ShieldCheck, UserRound } from "lucide-react";
import { BRAND_NAME, Logo } from "@/components/brand";
import { HeroMark, RotatingTagline } from "@/features/landing/hero";
import { Button } from "@/components/ui/button";
import { DISCLAIMER } from "@/lib/format";
import { LiveExample } from "@/features/landing/live-example";

const steps = [
  {
    title: "Профиль компании",
    text: "Отрасль, масштаб, банковские продукты, внешнеэкономическая деятельность и отрасли контрагентов — из данных банка. Вы дополняете то, что знаете только вы.",
  },
  {
    title: "Отбор событий",
    text: "Новости из официальных лент и материалы аналитиков Альфа-Банка сопоставляются с профилем компании и вашей ролью.",
  },
  {
    title: "Персональный анализ",
    text: "Агент строит план, ищет подтверждения в аналитике банка, пишет вывод и сам проверяет его по источникам.",
  },
];

const values = [
  {
    icon: UserRound,
    title: "Разный вывод для разных ролей",
    text: "Одна новость о ставке для финансового директора — про стоимость долга, для руководителя закупок — про условия поставщиков.",
  },
  {
    icon: FileSearch,
    title: "Каждый вывод — со ссылкой",
    text: "Факты из новости, позиция банка и выводы модели разделены и привязаны к первоисточникам.",
  },
  {
    icon: Building2,
    title: "Контекст вашей компании",
    text: "Учитываются кредиты, лизинг, страны ВЭД и отрасли контрагентов, а не только тренды отрасли.",
  },
  {
    icon: ShieldCheck,
    title: "Честные ограничения",
    text: "Если данных не хватает, анализ прямо говорит, что является предположением.",
  },
];

export default function Landing() {
  return (
    <div className="min-h-dvh">
      <section className="relative overflow-hidden">
        <div aria-hidden className="app-backdrop absolute inset-0" />
        <div className="relative mx-auto max-w-[1240px] px-4 pb-20 pt-6 sm:px-8 lg:pb-28">
          <nav className="flex items-center justify-between">
            <Logo />
            <div className="flex items-center gap-2">
              <Button asChild variant="ghost" size="sm">
                <Link href="/login">Войти</Link>
              </Button>
              <Button asChild size="sm">
                <Link href="/register">Регистрация</Link>
              </Button>
            </div>
          </nav>
          <div className="mt-14 grid items-center gap-10 lg:mt-20 lg:grid-cols-[1.1fr_1fr]">
          <div className="max-w-[760px]">
            <h1 className="display text-[48px] sm:text-[72px] lg:text-[88px]">{BRAND_NAME}</h1>
            <RotatingTagline />
            <p className="mt-4 max-w-[52ch] text-[16px] leading-relaxed text-ink-soft">
              Сервис для руководителей крупного бизнеса: отбирает значимые события, объясняет их влияние с учётом вашей роли и
              опирается на аналитику Альфа-Банка.
            </p>
            <div className="mt-9 flex flex-wrap gap-3">
              <Button asChild size="lg">
                <Link href="/register">Создать аккаунт</Link>
              </Button>
              <Button asChild size="lg" variant="secondary">
                <Link href="/login">У меня есть аккаунт</Link>
              </Button>
            </div>
          </div>
          <HeroMark />
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-[1240px] px-4 py-16 sm:px-8 lg:py-24">
        <h2 className="display max-w-[18ch] text-[34px] sm:text-[44px]">Как появляется персональный вывод</h2>
        <ol className="mt-10 grid gap-8 md:grid-cols-3">
          {steps.map((s, i) => (
            <li key={s.title} className="border-t border-line pt-5">
              <span className="text-[13px] font-medium text-steel tabular">Шаг {i + 1}</span>
              <h3 className="mt-2 text-[19px] font-semibold tracking-[-0.01em]">{s.title}</h3>
              <p className="mt-2 text-[15px] leading-relaxed text-ink-soft">{s.text}</p>
            </li>
          ))}
        </ol>
      </section>

      <section className="bg-frost/60">
        <div className="mx-auto grid max-w-[1240px] gap-10 px-4 py-16 sm:px-8 lg:grid-cols-[1fr_1.4fr] lg:py-24">
          <div>
            <h2 className="display text-[34px] sm:text-[44px]">Не пересказ, а интерпретация</h2>
            <p className="mt-4 max-w-[46ch] text-[16px] leading-relaxed text-ink-soft">
              Так выглядит работа сервиса прямо сейчас: одна и та же новость, разобранная для разных ролей, и свежие
              поступления из официальных источников и аналитики Альфа-Банка.
            </p>
          </div>
          <LiveExample />
        </div>
      </section>

      <section className="mx-auto max-w-[1240px] px-4 py-16 sm:px-8 lg:py-24">
        <div className="grid gap-x-10 gap-y-8 sm:grid-cols-2">
          {values.map((v) => (
            <div key={v.title} className="flex gap-4">
              <v.icon className="mt-0.5 size-6 shrink-0 text-steel" aria-hidden />
              <div>
                <h3 className="text-[17px] font-semibold">{v.title}</h3>
                <p className="mt-1 text-[15px] leading-relaxed text-ink-soft">{v.text}</p>
              </div>
            </div>
          ))}
        </div>
        <div className="mt-14 flex flex-wrap items-center justify-between gap-4 border-t border-line pt-8">
          <p className="max-w-[60ch] text-[14px] leading-relaxed text-ink-soft">{DISCLAIMER}</p>
          <Button asChild>
            <Link href="/register">Начать</Link>
          </Button>
        </div>
      </section>
    </div>
  );
}
