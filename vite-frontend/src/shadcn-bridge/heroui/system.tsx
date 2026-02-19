import * as React from "react";

export interface HeroUIProviderProps {
  children: React.ReactNode;
  navigate?: unknown;
  useHref?: unknown;
}

export function HeroUIProvider({ children }: HeroUIProviderProps) {
  return <>{children}</>;
}
