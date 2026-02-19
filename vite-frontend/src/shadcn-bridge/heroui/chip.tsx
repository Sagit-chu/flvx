import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type ChipColor = "default" | "primary" | "secondary" | "success" | "warning" | "danger";
type ChipVariant = "solid" | "flat" | "light" | "bordered";
type ChipSize = "sm" | "md" | "lg";

export interface ChipProps extends React.ComponentProps<"span"> {
  color?: ChipColor;
  size?: ChipSize;
  variant?: ChipVariant;
}

function colorVariant(color: ChipColor) {
  if (color === "danger") {
    return "destructive" as const;
  }
  if (color === "success") {
    return "success" as const;
  }
  if (color === "warning") {
    return "warning" as const;
  }
  if (color === "secondary") {
    return "secondary" as const;
  }

  return "default" as const;
}

export function Chip({
  children,
  className,
  color = "default",
  size = "md",
  variant = "solid",
  ...props
}: ChipProps) {
  return (
    <Badge
      className={cn(
        variant === "flat" || variant === "light" ? "bg-opacity-15" : "",
        variant === "bordered" ? "border-current bg-transparent" : "",
        size === "sm" ? "px-2 py-0 text-[10px]" : "",
        size === "lg" ? "px-3 py-1 text-sm" : "",
        className,
      )}
      variant={colorVariant(color)}
      {...props}
    >
      {children}
    </Badge>
  );
}
