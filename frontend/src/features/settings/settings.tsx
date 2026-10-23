"use client";

import { useState } from "react";
import { Camera } from "lucide-react";
import { toast } from "sonner";
import { useMe } from "@/components/app-shell";
import { Avatar } from "@/components/avatar";
import { Button, buttonVariants } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Panel, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import type { Personalization, Role, Topic } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

export function ProfileTab() {
  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <ProfileForm />
      <div className="space-y-6">
        <Interests />
        <PersonalizationForm />
      </div>
    </div>
  );
}

function ProfileForm() {
  const { me, reload } = useMe();
  const roles = useApi<{ items: Role[] }>("/roles");
  const [form, setForm] = useState({
    full_name: me.full_name,
    role_code: me.role_code,
    position_title: me.role_code === "other" ? me.position_title : "",
    work_focus: me.work_focus,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (form.role_code === "other" && form.position_title.trim().length < 2) {
      setErrors({ position_title: "Опишите вашу роль" });
      return;
    }
    setBusy(true);
    setErrors({});
    try {
      await api("/me/profile", {
        method: "PUT",
        body: { ...form, position_title: form.role_code === "other" ? form.position_title : "" },
      });
      toast.success("Профиль сохранён");
      reload();
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      else toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Мой профиль">
      <AvatarUpload />
      <form onSubmit={save} className="mt-5 space-y-4">
        <Field id="s-name" label="Имя и фамилия" error={errors.full_name} required>
          <Input value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
        </Field>
        <Field id="s-email" label="Email" hint="Изменение email пока не поддерживается">
          <Input value={me.email} disabled readOnly />
        </Field>
        <Field id="s-role" label="Роль в принятии решений" hint="Определяет, через какую линзу модель смотрит на новости" error={errors.role_code} required>
          <Select value={form.role_code} onChange={(e) => setForm({ ...form, role_code: e.target.value })}>
            {roles.data?.items.map((r) => (
              <option key={r.code} value={r.code}>
                {r.title}
              </option>
            ))}
          </Select>
        </Field>
        {form.role_code === "other" && (
          <Field id="s-pos" label="Ваша роль в компании" error={errors.position_title} required>
            <Input value={form.position_title} onChange={(e) => setForm({ ...form, position_title: e.target.value })} />
          </Field>
        )}
        <Field id="s-focus" label="Рабочие интересы" error={errors.work_focus}>
          <Textarea value={form.work_focus} onChange={(e) => setForm({ ...form, work_focus: e.target.value })} maxLength={1000} />
        </Field>
        <div className="flex justify-end">
          <Button type="submit" loading={busy}>
            Сохранить профиль
          </Button>
        </div>
      </form>
    </Panel>
  );
}

// Фото профиля: уменьшается в браузере до 256 px и сохраняется как JPEG.
function AvatarUpload() {
  const { me, reload } = useMe();
  const [busy, setBusy] = useState(false);
  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (!/^image\/(jpeg|png|webp)$/.test(file.type)) {
      toast.error("Поддерживаются JPEG, PNG и WebP");
      return;
    }
    setBusy(true);
    try {
      const dataUrl = await resizeImage(file, 256);
      await api("/me/avatar", { method: "PUT", body: { data_url: dataUrl } });
      toast.success("Фото обновлено");
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    setBusy(true);
    try {
      await api("/me/avatar", { method: "DELETE" });
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="flex items-center gap-4">
      <Avatar src={me.avatar} name={me.full_name} size={72} />
      <div className="flex flex-wrap gap-2">
        <label className={cn(buttonVariants({ variant: "outline", size: "sm" }), "cursor-pointer", busy && "pointer-events-none opacity-60")}>
          <Camera /> {me.avatar ? "Заменить фото" : "Добавить фото"}
          <input type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" onChange={onFile} />
        </label>
        {me.avatar && (
          <Button variant="ghost" size="sm" onClick={remove} disabled={busy}>
            Удалить
          </Button>
        )}
      </div>
    </div>
  );
}

function resizeImage(file: File, size: number): Promise<string> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    const url = URL.createObjectURL(file);
    img.onload = () => {
      const scale = Math.min(1, size / Math.min(img.width, img.height));
      const w = Math.round(img.width * scale);
      const h = Math.round(img.height * scale);
      const side = Math.min(w, h);
      const canvas = document.createElement("canvas");
      canvas.width = side;
      canvas.height = side;
      const ctx = canvas.getContext("2d");
      if (!ctx) return reject(new Error("canvas"));
      ctx.drawImage(img, (w - side) / -2, (h - side) / -2, w, h);
      URL.revokeObjectURL(url);
      resolve(canvas.toDataURL("image/jpeg", 0.85));
    };
    img.onerror = () => reject(new Error("Не удалось прочитать изображение"));
    img.src = url;
  });
}

const weights = [
  { v: 0, l: "Не важно" },
  { v: 1, l: "Иногда" },
  { v: 2, l: "Важно" },
  { v: 3, l: "Очень важно" },
];

function Interests() {
  const { me, reload } = useMe();
  const topics = useApi<{ items: Topic[] }>("/topics");
  const [w, setW] = useState<Record<string, number>>(() => Object.fromEntries(me.interests.map((i) => [i.topic_code, i.weight])));
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api("/me/interests", {
        method: "PUT",
        body: { topics: Object.entries(w).filter(([, v]) => v > 0).map(([code, weight]) => ({ code, weight })) },
      });
      toast.success("Интересы сохранены", { description: "Оценки ленты будут пересчитаны." });
      reload();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Темы интересов" description="Вес темы влияет на предварительную оценку и передаётся модели">
      {!topics.data ? (
        <Skeleton className="h-40" />
      ) : (
        <ul className="max-h-[340px] divide-y divide-line/70 overflow-y-auto pr-1">
          {topics.data.items.map((t) => (
            <li key={t.code} className="flex items-center justify-between gap-3 py-2">
              <span className="text-[14px]">{t.name}</span>
              <div className="flex shrink-0 gap-1" role="radiogroup" aria-label={`Важность темы ${t.name}`}>
                {weights.map((x) => (
                  <button
                    key={x.v}
                    role="radio"
                    aria-checked={(w[t.code] ?? 0) === x.v}
                    title={x.l}
                    onClick={() => setW({ ...w, [t.code]: x.v })}
                    className={cn(
                      "size-6 rounded-full border text-[11px] tabular transition-colors",
                      (w[t.code] ?? 0) === x.v ? "border-steel bg-steel text-white" : "border-line bg-white/60 text-silver hover:border-glacier",
                    )}
                  >
                    {x.v}
                  </button>
                ))}
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-4 flex items-center justify-between">
        <p className="text-[12px] text-silver">0 — не важно, 3 — очень важно</p>
        <Button onClick={save} loading={busy}>
          Сохранить интересы
        </Button>
      </div>
    </Panel>
  );
}

function PersonalizationForm() {
  const { me, reload } = useMe();
  const [p, setP] = useState<Personalization>(me.personalization);
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api("/me/personalization", { method: "PUT", body: p });
      toast.success("Настройки персонализации сохранены");
      reload();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Персонализация анализа">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="p-h" label="Горизонт планирования">
          <Select value={p.horizon} onChange={(e) => setP({ ...p, horizon: e.target.value as Personalization["horizon"] })}>
            <option value="short">До 3 месяцев</option>
            <option value="medium">3–12 месяцев</option>
            <option value="long">Больше года</option>
          </Select>
        </Field>
        <Field id="p-d" label="Детализация анализа">
          <Select value={p.detail} onChange={(e) => setP({ ...p, detail: e.target.value as Personalization["detail"] })}>
            <option value="detailed">Подробно</option>
            <option value="brief">Кратко</option>
          </Select>
        </Field>
      </div>
      <label className="mt-4 flex items-start gap-3 text-[14px]">
        <input
          type="checkbox"
          className="mt-0.5 size-4 accent-steel"
          checked={p.include_research}
          onChange={(e) => setP({ ...p, include_research: e.target.checked })}
        />
        <span>
          Поднимать в ленте материалы аналитиков Альфа-Банка
          <span className="block text-[12px] text-silver">Если темы материала совпадают с вашим профилем</span>
        </span>
      </label>
      <div className="mt-4 flex justify-end">
        <Button onClick={save} loading={busy}>
          Сохранить
        </Button>
      </div>
    </Panel>
  );
}
