import type { ReactNode } from "react";
import "./globals.css";
import SessionBoot from "@/components/SessionBoot";
import { ServiceWorkerBoot } from "@/components/ServiceWorkerBoot";

export const metadata = {
  title: "AutoDJ — Player",
  description: "Radio-as-software: now-playing e fila ao vivo",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="pt-BR">
      <body>
        <ServiceWorkerBoot />
        <SessionBoot>{children}</SessionBoot>
      </body>
    </html>
  );
}
