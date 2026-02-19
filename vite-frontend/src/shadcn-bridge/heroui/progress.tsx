import * as React from "react";

import { Progress as BaseProgress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";

type ProgressColor = "default" | "primary" | "secondary" | "success" | "warning" | "danger";

export interface ProgressProps {
  "aria-label"?: string;
  className?: string;
  color?: ProgressColor;
  label?: React.ReactNode;
  showValueLabel?: boolean;
  size?: "sm" | "md" | "lg";
  value?: number;
}

function indicatorClassName(color: ProgressColor) {
  if (color === "danger") {
    return "bg-danger";
  }
  if (color === "success") {
    return "bg-success";
  }
  if (color === "warning") {
    return "bg-warning";
  }

  return "bg-primary";
}

export function Progress({
  "aria-label": ariaLabel,
  className,
  color = "primary",
  label,
  showValueLabel,
  size = "md",
  value = 0,
}: ProgressProps) {
  return (
    <div className={cn("w-full space-y-1", className)}>
      {label || showValueLabel ? (
        <div className="flex items-center justify-between text-xs text-default-500">
          <span>{label}</span>
          {showValueLabel ? <span>{Math.round(value)}%</span> : null}
        </div>
      ) : null}
      <BaseProgress
        aria-label={ariaLabel}
        className={cn(size === "sm" ? "h-1.5" : "", size === "lg" ? "h-3" : "")}
        indicatorClassName={indicatorClassName(color)}
        value={value}
      />
    </div>
  );
}
