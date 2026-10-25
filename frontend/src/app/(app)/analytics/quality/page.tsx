import type { Metadata } from "next";
import { QualityView } from "@/features/analytics/quality";

export const metadata: Metadata = { title: "Качество AI" };

export default function QualityPage() {
  return <QualityView />;
}
