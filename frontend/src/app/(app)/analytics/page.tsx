import type { Metadata } from "next";
import { AnalyticsView } from "@/features/analytics/analytics";

export const metadata: Metadata = { title: "Аналитика" };

export default function AnalyticsPage() {
  return <AnalyticsView />;
}
