import * as React from "react";
import { Loader2Icon } from "lucide-react";

import { Button as BaseButton } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type HeroButtonColor = "default" | "primary" | "secondary" | "success" | "warning" | "danger";
type HeroButtonVariant = "solid" | "light" | "flat" | "ghost" | "bordered" | "shadow";
type HeroButtonSize = "sm" | "md" | "lg";

export interface ButtonProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "color"> {
  color?: HeroButtonColor;
  endContent?: React.ReactNode;
  isIconOnly?: boolean;
  isLoading?: boolean;
  onPress?: (event: React.MouseEvent<HTMLButtonElement>) => void;
  size?: HeroButtonSize;
  startContent?: React.ReactNode;
  variant?: HeroButtonVariant;
}

function mapVariant(color: HeroButtonColor, variant: HeroButtonVariant): "default" | "destructive" | "secondary" | "outline" | "ghost" | "light" | "flat" {
  if (variant === "bordered") {
    return "outline";
  }
  if (variant === "ghost") {
    return "ghost";
  }
  if (variant === "light") {
    return "light";
  }
  if (variant === "flat") {
    return "flat";
  }
  if (color === "danger") {
    return "destructive";
  }
  if (color === "secondary") {
    return "secondary";
  }

  return "default";
}

function mapSize(size: HeroButtonSize, isIconOnly: boolean): "default" | "sm" | "lg" | "icon" {
  if (isIconOnly) {
    return "icon";
  }
  if (size === "sm") {
    return "sm";
  }
  if (size === "lg") {
    return "lg";
  }

  return "default";
}

export function Button({
  children,
  className,
  color = "default",
  disabled,
  endContent,
  isIconOnly = false,
  isLoading = false,
  isDisabled,
  onClick,
  onPress,
  size = "md",
  startContent,
  type = "button",
  variant = "solid",
  ...props
}: ButtonProps & {
  isDisabled?: boolean;
}) {
  const resolvedVariant = mapVariant(color, variant);
  const resolvedSize = mapSize(size, isIconOnly);
  const resolvedDisabled = Boolean(disabled || isDisabled || isLoading);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    onClick?.(event);
    onPress?.(event);
  };

  return (
    <BaseButton
      className={cn(isIconOnly ? "p-0" : "", className)}
      disabled={resolvedDisabled}
      size={resolvedSize}
      type={type}
      variant={resolvedVariant}
      onClick={handleClick}
      {...props}
    >
      {isLoading ? <Loader2Icon className="mr-2 h-4 w-4 animate-spin" /> : null}
      {startContent}
      {isIconOnly ? null : children}
      {isIconOnly ? children : null}
      {endContent}
    </BaseButton>
  );
}
