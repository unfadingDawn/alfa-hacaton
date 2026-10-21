import type { Metadata } from "next";
import { CompanyView } from "@/features/company/company";

export const metadata: Metadata = { title: "Компания" };

export default function CompanyPage() {
  return <CompanyView />;
}
