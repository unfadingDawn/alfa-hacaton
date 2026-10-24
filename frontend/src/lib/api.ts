// Клиент API. Все запросы идут на тот же домен (/api/*), Next.js проксирует их
// в Go-backend. Сессия — HttpOnly cookie, поэтому токены в JS недоступны.

export class ApiError extends Error {
  status: number;
  code: string;
  fields?: Record<string, string>;
  constructor(status: number, code: string, message: string, fields?: Record<string, string>) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

let refreshing: Promise<boolean> | null = null;

async function refreshSession(): Promise<boolean> {
  if (!refreshing) {
    refreshing = fetch("/api/auth/refresh", { method: "POST", credentials: "same-origin" })
      .then((r) => r.ok)
      .catch(() => false)
      .finally(() => {
        setTimeout(() => (refreshing = null), 0);
      });
  }
  return refreshing;
}

type Options = { method?: string; body?: unknown; signal?: AbortSignal; retry?: boolean };

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
  const { method = "GET", body, signal, retry = true } = opts;
  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method,
      signal,
      credentials: "same-origin",
      headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    if ((e as Error).name === "AbortError") throw e;
    throw new ApiError(0, "network_error", "Нет связи с сервером. Проверьте подключение и попробуйте ещё раз.");
  }
  if (res.status === 401 && retry && !path.startsWith("/auth/")) {
    if (await refreshSession()) return api<T>(path, { ...opts, retry: false });
    if (typeof window !== "undefined" && !window.location.pathname.startsWith("/login")) {
      window.location.href = `/login?next=${encodeURIComponent(window.location.pathname)}`;
    }
  }
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    const err = (data as { error?: { code: string; message: string; fields?: Record<string, string> } })?.error;
    if (err) throw new ApiError(res.status, err.code, err.message, err.fields);
    if (res.status >= 500) throw new ApiError(res.status, "server_error", "Сервер недоступен. Попробуйте позже.");
    throw new ApiError(res.status, "http_error", `Ошибка запроса (${res.status})`);
  }
  return data as T;
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return "Что-то пошло не так. Попробуйте ещё раз.";
}
