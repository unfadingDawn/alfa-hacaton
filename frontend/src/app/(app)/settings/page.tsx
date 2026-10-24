import { redirect } from "next/navigation";

// Настройки объединены с разделом «Компания» в «Личный кабинет».
export default function SettingsPage() {
  redirect("/cabinet");
}
