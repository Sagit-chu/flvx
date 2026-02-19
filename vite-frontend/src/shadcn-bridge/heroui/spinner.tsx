import * as React from "react";
import { Loader2Icon } from "lucide-react";

import { cn } from "@/lib/utils";

type SpinnerSize = "sm" | "md" | "lg";

export interface SpinnerProps extends React.ComponentProps<"div"> {
  label?: string;
  size?: SpinnerSize;
}

function iconSize(size: SpinnerSize | undefined) {
  if (size === "sm") {
    return "h-4 w-4";
  }
  if (size === "lg") {
    return "h-7 w-7";
  }

  return "h-5 w-5";
}

export function Spinner({ className, label, size = "md", ...props }: SpinnerProps) {
  return (
    <div className={cn("inline-flex items-center gap-2", className)} {...props}>
      <Loader2Icon className={cn("animate-spin text-default-500", iconSize(size))} />
      {label ? <span className="text-sm text-default-500">{label}</span> : null}
    </div>
  );
}
