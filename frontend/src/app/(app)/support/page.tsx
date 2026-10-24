import type { Metadata } from "next";
import { SupportView } from "@/features/support/support";

export const metadata: Metadata = { title: "Поддержка" };

export default function SupportPage() {
  return <SupportView />;
}
