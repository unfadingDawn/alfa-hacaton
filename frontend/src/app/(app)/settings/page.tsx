import type { Metadata } from "next";
import { SettingsView } from "@/features/settings/settings";

export const metadata: Metadata = { title: "Настройки" };

export default function SettingsPage() {
  return <SettingsView />;
}
