import type { Metadata } from "next";
import { HomeView } from "@/features/home/home";

export const metadata: Metadata = { title: "Главная" };

export default function HomePage() {
  return <HomeView />;
}
