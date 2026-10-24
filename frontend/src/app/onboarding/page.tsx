import type { Metadata } from "next";
import { Onboarding } from "@/features/onboarding/onboarding";

export const metadata: Metadata = { title: "Настройка профиля" };

export default function OnboardingPage() {
  return <Onboarding />;
}
