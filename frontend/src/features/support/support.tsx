"use client";

import { Headset, Paperclip, Send } from "lucide-react";
import { PageHeader } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/field";

// Раздел поддержки: пока только оформление — специалист поддержки ещё не подключён.
export function SupportView() {
  return (
    <div>
      <PageHeader title="Поддержка" lead="Здесь можно будет задать вопрос специалисту по работе сервиса." />
      <section className="glass flex min-h-[520px] flex-col overflow-hidden rounded-[var(--radius-xl)]" aria-label="Чат поддержки">
        <header className="flex items-center gap-3 border-b border-line/80 px-5 py-4">
          <span className="flex size-10 items-center justify-center rounded-full bg-steel/10 text-steel">
            <Headset className="size-5" aria-hidden />
          </span>
          <div>
            <p className="text-[15px] font-medium">Поддержка «Альфа Инсайт»</p>
            <p className="text-[12px] text-silver">Пока не подключена</p>
          </div>
        </header>
        <div className="flex flex-1 items-center justify-center px-6 py-10 text-center">
          <div className="max-w-[40ch]">
            <p className="text-[17px] font-medium">Чат скоро появится</p>
            <p className="mt-2 text-[14px] text-ink-soft">
              Специалист поддержки ещё не подключён. Когда он появится, здесь будет история обращений и ответы.
            </p>
          </div>
        </div>
        <footer className="flex items-center gap-2 border-t border-line/80 p-3">
          <Button variant="ghost" size="icon" disabled aria-label="Прикрепить файл">
            <Paperclip />
          </Button>
          <Input disabled placeholder="Сообщение" aria-label="Сообщение в поддержку" className="flex-1" />
          <Button size="icon" disabled aria-label="Отправить">
            <Send />
          </Button>
        </footer>
      </section>
    </div>
  );
}
