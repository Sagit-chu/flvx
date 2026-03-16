import * as React from "react";
import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { CheckIcon, MinusIcon } from "lucide-react";

import { cn } from "@/lib/utils";

function Checkbox({
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      className={cn(
        "peer h-4 w-4 shrink-0 rounded-sm border border-primary shadow transition-transform duration-100 active:scale-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground data-[state=checked]:shadow-none data-[state=indeterminate]:shadow-none",
        className,
      )}
      data-slot="checkbox"
      {...props}
    >
      <CheckboxPrimitive.Indicator
        className="flex items-center justify-center text-current data-[state=checked]:animate-in data-[state=checked]:zoom-in-75 data-[state=checked]:duration-150 data-[state=indeterminate]:animate-in data-[state=indeterminate]:zoom-in-75 data-[state=indeterminate]:duration-150"
        data-slot="checkbox-indicator"
      >
        <CheckIcon className="h-3.5 w-3.5 data-[state=indeterminate]:hidden" />
        <MinusIcon className="h-3.5 w-3.5 hidden data-[state=indeterminate]:block" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

export { Checkbox };
