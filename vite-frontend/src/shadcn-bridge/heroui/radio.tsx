import * as React from "react";

import { RadioGroup as BaseRadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

interface RadioContextValue {
  name: string;
}

const RadioContext = React.createContext<RadioContextValue>({ name: "radio-group" });

export interface RadioGroupProps {
  children: React.ReactNode;
  label?: React.ReactNode;
  onValueChange?: (value: string) => void;
  orientation?: "horizontal" | "vertical";
  value?: string;
}

export function RadioGroup({
  children,
  label,
  onValueChange,
  orientation = "vertical",
  value,
}: RadioGroupProps) {
  const generatedName = React.useId();

  return (
    <div className="space-y-2">
      {label ? <p className="text-sm font-medium">{label}</p> : null}
      <RadioContext.Provider value={{ name: generatedName }}>
        <BaseRadioGroup
          className={cn(
            orientation === "horizontal" ? "flex flex-wrap items-center gap-4" : "grid gap-3",
          )}
          value={value}
          onValueChange={onValueChange}
        >
          {children}
        </BaseRadioGroup>
      </RadioContext.Provider>
    </div>
  );
}

export interface RadioProps {
  children?: React.ReactNode;
  value: string;
}

export function Radio({ children, value }: RadioProps) {
  const context = React.useContext(RadioContext);
  const id = `${context.name}-${value}`;

  return (
    <div className="flex items-center gap-2">
      <RadioGroupItem id={id} value={value} />
      <Label htmlFor={id}>{children}</Label>
    </div>
  );
}
