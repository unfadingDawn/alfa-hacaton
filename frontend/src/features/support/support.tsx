"use client";

import { PageHeader } from "@/components/app-shell";
import { SupportChat } from "./chat";

// Полноэкранная версия ИИ-помощника. Обычно он открывается плавающей кнопкой
// в правом нижнем углу; эта страница — для прямой ссылки и длинных диалогов.
export function SupportView() {
  return (
    <div>
      <PageHeader
        title="Поддержка"
        lead="ИИ-помощник отвечает на вопросы о работе сервиса по справке «Альфа Инсайт». Ответы сверяются со справкой, выдуманные шаги и разделы удаляются."
      />
      <section className="glass flex h-[min(720px,calc(100dvh-220px))] min-h-[480px] flex-col overflow-hidden rounded-[var(--radius-xl)]" aria-label="Чат с ИИ-помощником">
        <SupportChat autoFocus />
      </section>
    </div>
  );
}
