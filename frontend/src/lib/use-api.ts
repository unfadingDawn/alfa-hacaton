"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";

type State<T> = { data: T | null; error: unknown; loading: boolean };

// Простой хук загрузки данных: состояние загрузки, ошибка, повтор.
export function useApi<T>(path: string | null) {
  const [state, setState] = useState<State<T>>({ data: null, error: null, loading: !!path });
  const ctrl = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    if (!path) return;
    ctrl.current?.abort();
    const c = new AbortController();
    ctrl.current = c;
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const data = await api<T>(path, { signal: c.signal });
      if (!c.signal.aborted) setState({ data, error: null, loading: false });
    } catch (e) {
      if ((e as Error).name === "AbortError") return;
      setState((s) => ({ ...s, error: e, loading: false }));
    }
  }, [path]);

  useEffect(() => {
    void load();
    return () => ctrl.current?.abort();
  }, [load]);

  const mutate = useCallback((fn: (d: T | null) => T | null) => setState((s) => ({ ...s, data: fn(s.data) })), []);

  return { ...state, reload: load, mutate };
}
