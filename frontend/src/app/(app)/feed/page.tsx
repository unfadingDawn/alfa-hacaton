import type { Metadata } from "next";
import { Suspense } from "react";
import { Feed } from "@/features/news/feed";

export const metadata: Metadata = { title: "Лента" };

export default function FeedPage() {
  return (
    <Suspense>
      <Feed />
    </Suspense>
  );
}
