import * as React from "react";

import { cn } from "@/lib/utils";

type NavbarPosition = "static" | "sticky";
type MaxWidth = "sm" | "md" | "lg" | "xl" | "2xl" | "full";

export interface NavbarProps extends React.ComponentProps<"nav"> {
  height?: string;
  maxWidth?: MaxWidth;
  position?: NavbarPosition;
}

function maxWidthClass(maxWidth: MaxWidth | undefined) {
  if (maxWidth === "sm") {
    return "max-w-screen-sm";
  }
  if (maxWidth === "md") {
    return "max-w-screen-md";
  }
  if (maxWidth === "lg") {
    return "max-w-screen-lg";
  }
  if (maxWidth === "xl") {
    return "max-w-screen-xl";
  }
  if (maxWidth === "2xl") {
    return "max-w-screen-2xl";
  }
  if (maxWidth === "full") {
    return "max-w-none";
  }

  return "max-w-screen-xl";
}

export function Navbar({
  children,
  className,
  height,
  maxWidth = "xl",
  position = "static",
  ...props
}: NavbarProps) {
  return (
    <nav
      className={cn(
        "w-full border-b border-default-200 bg-white/90 backdrop-blur dark:bg-default-50/60",
        position === "sticky" ? "sticky top-0 z-40" : "",
        className,
      )}
      style={height ? { minHeight: height } : undefined}
      {...props}
    >
      <div
        className={cn(
          "mx-auto flex h-full w-full items-center justify-between px-4",
          maxWidthClass(maxWidth),
        )}
      >
        {children}
      </div>
    </nav>
  );
}

export interface NavbarContentProps extends React.ComponentProps<"div"> {
  justify?: "start" | "center" | "end";
}

export function NavbarContent({
  children,
  className,
  justify = "start",
  ...props
}: NavbarContentProps) {
  return (
    <div
      className={cn(
        "flex flex-1 items-center gap-2",
        justify === "center" ? "justify-center" : "",
        justify === "end" ? "justify-end" : "",
        justify === "start" ? "justify-start" : "",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export function NavbarBrand({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return <div className={cn("flex items-center", className)} {...props} />;
}
