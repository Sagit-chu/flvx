import * as React from "react";

import { Checkbox as BaseCheckbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";

export interface CheckboxProps
  extends Omit<
    React.ComponentProps<typeof BaseCheckbox>,
    "checked" | "onCheckedChange"
  > {
  classNames?: Record<string, string>;
  color?: string;
  isDisabled?: boolean;
  isSelected?: boolean;
  onValueChange?: (value: boolean) => void;
  size?: string;
}

export function Checkbox({
  children,
  className,
  isDisabled,
  isSelected,
  onValueChange,
  ...props
}: CheckboxProps & {
  children?: React.ReactNode;
}) {
  const handleCheckedChange: React.ComponentProps<
    typeof BaseCheckbox
  >["onCheckedChange"] = (value: boolean | "indeterminate") => {
    onValueChange?.(value === true);
  };

  return (
    <div
      className={cn(
        "inline-flex items-center gap-2",
        isDisabled ? "opacity-50" : "",
        className,
      )}
    >
      <BaseCheckbox
        checked={Boolean(isSelected)}
        disabled={isDisabled}
        onCheckedChange={handleCheckedChange}
        {...props}
      />
      {children ? <span className="text-sm">{children}</span> : null}
    </div>
  );
}
