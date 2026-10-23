import { redirect } from "next/navigation";

// Раздел «Компания» объединён с настройками в «Личный кабинет».
export default function CompanyPage() {
  redirect("/cabinet?tab=company");
}
