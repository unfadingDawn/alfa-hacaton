"use client";

import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { AuthShell } from "@/components/auth-shell";
import { Stepper } from "@/components/stepper";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { api, ApiError, errorMessage } from "@/lib/api";

export default function RegisterPage() {
  const router = useRouter();
  const [form, setForm] = useState({ full_name: "", email: "", password: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);

  function validate() {
    const e: Record<string, string> = {};
    if (form.full_name.trim().length < 2) e.full_name = "Укажите имя";
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email.trim())) e.email = "Введите корректный email";
    if (form.password.length < 8) e.password = "Не менее 8 символов";
    else if (!/\d/.test(form.password) || !/\p{L}/u.test(form.password)) e.password = "Пароль должен содержать буквы и цифры";
    return e;
  }

  async function submit(ev: React.FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    setFormError("");
    if (Object.keys(e).length) return;
    setLoading(true);
    try {
      await api("/auth/register", { method: "POST", body: { ...form, email: form.email.trim() } });
      router.replace("/onboarding");
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      else setFormError(errorMessage(err));
      setLoading(false);
    }
  }

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value });

  return (
    <AuthShell
      headline={<>Аналитика, которая знает вашу компанию</>}
      lead="Регистрация займёт пару минут: создайте аккаунт, выберите компанию и проверьте данные. Дальше лента подстроится под вашу роль."
    >
      <form onSubmit={submit} noValidate className="flex flex-col gap-4">
        <Stepper current={0} className="mb-2" />
        <div>
          <h2 className="text-[22px] font-semibold tracking-[-0.02em]">Создайте аккаунт</h2>
          <p className="mt-1 text-sm text-ink-soft">Пароль хранится только в виде хеша.</p>
        </div>
        <Field id="full_name" label="Имя и фамилия" error={errors.full_name}>
          <Input autoComplete="name" value={form.full_name} onChange={set("full_name")} />
        </Field>
        <Field id="email" label="Рабочий email" error={errors.email}>
          <Input type="email" autoComplete="email" value={form.email} onChange={set("email")} placeholder="name@company.ru" />
        </Field>
        <Field id="password" label="Пароль" hint="Не менее 8 символов, буквы и цифры" error={errors.password}>
          <Input type="password" autoComplete="new-password" value={form.password} onChange={set("password")} />
        </Field>
        {formError && (
          <p role="alert" className="rounded-[10px] bg-risk-bg px-3 py-2 text-sm text-risk">
            {formError}
          </p>
        )}
        <Button type="submit" size="lg" loading={loading} className="mt-2 w-full">
          Продолжить
        </Button>
        <p className="text-center text-sm text-ink-soft">
          Уже есть аккаунт?{" "}
          <Link href="/login" className="font-medium text-steel hover:underline">
            Войти
          </Link>
        </p>
      </form>
    </AuthShell>
  );
}
