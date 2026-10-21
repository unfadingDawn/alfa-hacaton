"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AuthShell } from "@/components/auth-shell";
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
      <p className="text-center text-sm text-ink-soft">
        Нет аккаунта?{" "}
        <Link href="/register" className="font-medium text-steel hover:underline">
          Зарегистрироваться
        </Link>
      </p>
    </form>
  );
}

export default function LoginPage() {
  return (
    <AuthShell
      headline={<>Новости, объяснённые для вашего бизнеса</>}
      lead="AlfaInsight отбирает события, значимые для вашей компании, и объясняет их влияние с учётом вашей роли и аналитики Альфа-Банка."
    >
      <Suspense>
        <LoginForm />
      </Suspense>
    </AuthShell>
  );
}
