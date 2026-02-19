import * as React from "react";

import { Switch as BaseSwitch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

export interface SwitchProps extends Omit<React.ComponentProps<typeof BaseSwitch>, "checked" | "onCheckedChange"> {
  classNames?: Record<string, string>;
  color?: string;
  isDisabled?: boolean;
  isSelected?: boolean;
  onValueChange?: (value: boolean) => void;
  size?: string;
}

export function Switch({
  className,
  isDisabled,
  isSelected,
  onValueChange,
  ...props
}: SwitchProps) {
  return (
    <BaseSwitch
      checked={Boolean(isSelected)}
      className={cn(className)}
      disabled={isDisabled}
      onCheckedChange={onValueChange}
      {...props}
    />
  );
}
