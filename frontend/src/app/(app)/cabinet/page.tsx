import type { Metadata } from "next";
import { Suspense } from "react";
import { Cabinet } from "@/features/cabinet/cabinet";

export const metadata: Metadata = { title: "Личный кабинет" };

export default function CabinetPage() {
  return (
    <Suspense>
      <Cabinet />
    </Suspense>
  );
}
