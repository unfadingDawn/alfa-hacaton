import type { Metadata } from "next";
import { Suspense } from "react";
import { Feed } from "@/features/news/feed";

export const metadata: Metadata = { title: "Сохранённые" };

export default function SavedPage() {
  return (
    <Suspense>
      <Feed savedOnly />
    </Suspense>
  );
}
