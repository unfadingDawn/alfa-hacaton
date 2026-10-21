import Link from "next/link";
import { Building2, FileSearch, ShieldCheck, UserRound } from "lucide-react";
import { Logo } from "@/components/brand";
import { Button } from "@/components/ui/button";

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
        <div aria-hidden className="absolute inset-0 bg-cover bg-[center_right]" style={{ backgroundImage: "url(/waves.jpg)" }} />
        <div aria-hidden className="absolute inset-0 bg-gradient-to-r from-ice via-ice/70 to-ice/5" />
        <div className="relative mx-auto max-w-[1240px] px-4 pb-24 pt-6 sm:px-8 lg:pb-36">
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
          <div className="mt-20 max-w-[760px] lg:mt-32">
            <h1 className="display text-[48px] sm:text-[72px] lg:text-[96px]">AlfaInsight</h1>
            <p className="mt-6 max-w-[34ch] text-[22px] leading-snug text-ink sm:text-[26px]">
              Что произошло в экономике — и что это значит именно для вашей компании.
            </p>
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
        <div className="mx-auto grid max-w-[1240px] gap-10 px-4 py-16 sm:px-8 lg:grid-cols-[1fr_1.2fr] lg:py-24">
          <div>
            <h2 className="display text-[34px] sm:text-[44px]">Не пересказ, а интерпретация</h2>
            <p className="mt-4 max-w-[46ch] text-[16px] leading-relaxed text-ink-soft">
              Пример из материалов кейса: компания с кредитными продуктами, новость — сохранение ключевой ставки.
            </p>
          </div>
          <figure className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">
            <p className="text-[13px] text-silver">Пересказ</p>
            <p className="mt-1 text-[15px] leading-relaxed text-ink-soft">
              «11 сентября 2026 года совет директоров Банка России принял решение сохранить ключевую ставку на уровне 14% годовых.»
            </p>
            <div className="my-5 h-px bg-line" />
            <p className="text-[13px] text-steel">Интерпретация для бизнеса</p>
            <blockquote className="mt-1 text-[18px] leading-snug text-ink">
              «Для бизнеса сохранение ключевой ставки на уровне 14% — сигнал о том, что период быстрого удешевления кредитов
              временно приостановлен. Деньги остаются дорогими, а значит, финансовая нагрузка на компании, зависящие от заёмного
              финансирования, не снизится в ближайшей перспективе.»
            </blockquote>
            <figcaption className="mt-4 text-[12px] text-silver">Источник: презентация кейса хакатона, раздел «Ограничения».</figcaption>
          </figure>
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
          <p className="text-[15px] text-ink-soft">Прототип для хакатона Альфа-Банка. Данные компаний обезличены.</p>
          <Button asChild>
            <Link href="/register">Начать</Link>
          </Button>
        </div>
      </section>
    </div>
  );
}
