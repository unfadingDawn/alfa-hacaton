"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Building2 } from "lucide-react";
import { AuthShell } from "@/components/auth-shell";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { api, ApiError, errorMessage } from "@/lib/api";

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const fe: Record<string, string> = {};
    if (!email.trim()) fe.email = "Введите email";
    if (!password) fe.password = "Введите пароль";
    setErrors(fe);
    setFormError("");
    if (Object.keys(fe).length) return;
    setLoading(true);
    try {
      const res = await api<{ onboarding_completed: boolean }>("/auth/login", { method: "POST", body: { email, password } });
      const next = params.get("next");
      router.replace(!res.onboarding_completed ? "/onboarding" : next && next.startsWith("/") ? next : "/home");
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      setFormError(errorMessage(err));
      setLoading(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <div className="mb-1">
        <h2 className="text-[22px] font-semibold tracking-[-0.02em]">Вход</h2>
        <p className="mt-1 text-sm text-ink-soft">Продолжите работу с персональной лентой.</p>
      </div>
      <Field id="email" label="Email" error={errors.email}>
        <Input type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@company.ru" />
      </Field>
      <Field id="password" label="Пароль" error={errors.password}>
        <Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      {formError && !Object.keys(errors).length && (
        <p role="alert" className="rounded-[10px] bg-risk-bg px-3 py-2 text-sm text-risk">
          {formError}
        </p>
      )}
      <Button type="submit" size="lg" loading={loading} className="mt-2 w-full">
        Войти
      </Button>
      <div className="flex items-center gap-3 text-[12px] text-silver" aria-hidden>
        <span className="h-px flex-1 bg-line" /> или <span className="h-px flex-1 bg-line" />
      </div>
      <AlfaBusinessLogin />
      <p className="text-center text-sm text-ink-soft">
        Нет аккаунта?{" "}
        <Link href="/register" className="font-medium text-steel hover:underline">
          Зарегистрироваться
        </Link>
      </p>
    </form>
  );
}

// Вход из интернет-банка «Альфа-Бизнес». Интеграция ещё не подключена —
// backend честно сообщает об этом (контракт описан в internal/integrations/alfabusiness).
function AlfaBusinessLogin() {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  async function start() {
    setBusy(true);
    try {
      const r = await api<{ connected: boolean; message?: string }>("/auth/alfa-business");
      if (r.connected) {
        window.location.href = "/api/auth/alfa-business/start";
        return;
      }
      setMessage(r.message ?? "");
      setOpen(true);
    } catch (e) {
      setMessage(errorMessage(e));
      setOpen(true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button type="button" size="lg" variant="dark" className="w-full" onClick={start} loading={busy}>
        {!busy && <Building2 />} Вход из Альфа-Бизнеса
      </Button>
      <DialogContent title="Вход из Альфа-Бизнеса" description="Данные профиля и компании подтянутся из личного кабинета интернет-банка.">
        <p className="text-sm text-ink-soft">{message}</p>
        <div className="mt-6 flex justify-end">
          <Button onClick={() => setOpen(false)}>Понятно</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default function LoginPage() {
  return (
    <AuthShell
      headline={<>Новости, объяснённые для вашего бизнеса</>}
      lead="Альфа Инсайт отбирает события, значимые для вашей компании, и объясняет их влияние с учётом вашей роли и аналитики Альфа-Банка."
    >
      <Suspense>
        <LoginForm />
      </Suspense>
    </AuthShell>
  );
}
