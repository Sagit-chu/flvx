import * as React from "react";
import { Loader2Icon } from "lucide-react";

import { Button as BaseButton } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type HeroButtonColor =
  | "default"
  | "primary"
  | "secondary"
  | "success"
  | "warning"
  | "danger";
type HeroButtonVariant =
  | "solid"
  | "light"
  | "flat"
  | "ghost"
  | "bordered"
  | "shadow";
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

function mapVariant(
  color: HeroButtonColor,
  variant: HeroButtonVariant,
):
  | "default"
  | "destructive"
  | "secondary"
  | "outline"
  | "ghost"
  | "light"
  | "flat" {
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

function mapSize(
  size: HeroButtonSize,
  isIconOnly: boolean,
): "default" | "sm" | "lg" | "icon" {
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

const solidColorClassMap: Partial<Record<HeroButtonColor, string>> = {
  default:
    "bg-default-200 text-default-800 hover:bg-default-300 dark:bg-default-100 dark:text-default-800 dark:hover:bg-default-200",
  success: "bg-success text-white hover:bg-success-600",
  warning: "bg-warning text-warning-900 hover:bg-warning-400",
};

const borderedColorClassMap: Record<HeroButtonColor, string> = {
  default:
    "border-default-300 text-default-700 hover:bg-default-100 dark:border-default-300 dark:text-default-600 dark:hover:bg-default-200/40",
  primary:
    "border-primary text-primary hover:bg-primary-50 dark:border-primary-500/60 dark:text-primary-300 dark:hover:bg-primary-900/20",
  secondary:
    "border-secondary text-secondary hover:bg-secondary-50 dark:border-secondary-500/60 dark:text-secondary-300 dark:hover:bg-secondary-900/20",
  success:
    "border-success text-success hover:bg-success-50 dark:border-success-500/60 dark:text-success-300 dark:hover:bg-success-900/20",
  warning:
    "border-warning text-warning-700 hover:bg-warning-50 dark:border-warning-500/60 dark:text-warning-300 dark:hover:bg-warning-900/20",
  danger:
    "border-danger text-danger hover:bg-danger-50 dark:border-danger-500/60 dark:text-danger-300 dark:hover:bg-danger-900/20",
};

const lightColorClassMap: Record<HeroButtonColor, string> = {
  default:
    "text-default-700 hover:bg-default-100 dark:text-default-600 dark:hover:bg-default-200/40",
  primary:
    "text-primary hover:bg-primary-100/70 dark:text-primary-300 dark:hover:bg-primary-900/30",
  secondary:
    "text-secondary hover:bg-secondary-100/70 dark:text-secondary-300 dark:hover:bg-secondary-900/30",
  success:
    "text-success hover:bg-success-100/70 dark:text-success-300 dark:hover:bg-success-900/30",
  warning:
    "text-warning-700 hover:bg-warning-100/70 dark:text-warning-300 dark:hover:bg-warning-900/30",
  danger:
    "text-danger hover:bg-danger-100/70 dark:text-danger-300 dark:hover:bg-danger-900/30",
};

const flatColorClassMap: Record<HeroButtonColor, string> = {
  default:
    "bg-default-100 text-default-700 hover:bg-default-200 dark:bg-default-100/45 dark:text-default-700 dark:hover:bg-default-200/60",
  primary:
    "bg-primary-100 text-primary-700 hover:bg-primary-200 dark:bg-primary-900/30 dark:text-primary-300 dark:hover:bg-primary-900/45",
  secondary:
    "bg-secondary-100 text-secondary-700 hover:bg-secondary-200 dark:bg-secondary-900/30 dark:text-secondary-300 dark:hover:bg-secondary-900/45",
  success:
    "bg-success-100 text-success-700 hover:bg-success-200 dark:bg-success-900/30 dark:text-success-300 dark:hover:bg-success-900/45",
  warning:
    "bg-warning-100 text-warning-700 hover:bg-warning-200 dark:bg-warning-900/30 dark:text-warning-300 dark:hover:bg-warning-900/45",
  danger:
    "bg-danger-100 text-danger-700 hover:bg-danger-200 dark:bg-danger-900/30 dark:text-danger-300 dark:hover:bg-danger-900/45",
};

const shadowColorClassMap: Record<HeroButtonColor, string> = {
  default: "shadow-md shadow-default-400/40",
  primary: "shadow-md shadow-primary-500/35",
  secondary: "shadow-md shadow-secondary-500/35",
  success: "shadow-md shadow-success-500/35",
  warning: "shadow-md shadow-warning-500/40",
  danger: "shadow-md shadow-danger-500/35",
};

function mapColorClass(
  color: HeroButtonColor,
  variant: HeroButtonVariant,
): string {
  if (variant === "bordered") {
    return borderedColorClassMap[color];
  }
  if (variant === "light") {
    return lightColorClassMap[color];
  }
  if (variant === "flat") {
    return flatColorClassMap[color];
  }
  if (variant === "solid" || variant === "shadow") {
    return solidColorClassMap[color] ?? "";
  }

  return "";
}

function mapShadowClass(
  color: HeroButtonColor,
  variant: HeroButtonVariant,
): string {
  if (variant !== "shadow") {
    return "";
  }

  return shadowColorClassMap[color];
}

export const Button = React.forwardRef<
  HTMLButtonElement,
  ButtonProps & {
    isDisabled?: boolean;
  }
>(
  (
    {
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
    },
    ref,
  ) => {
    const resolvedVariant = mapVariant(color, variant);
    const resolvedSize = mapSize(size, isIconOnly);
    const resolvedDisabled = Boolean(disabled || isDisabled || isLoading);
    const resolvedColorClass = mapColorClass(color, variant);
    const resolvedShadowClass = mapShadowClass(color, variant);

    const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
      onClick?.(event);
      onPress?.(event);
    };

    return (
      <BaseButton
        ref={ref}
        className={cn(
          isIconOnly ? "p-0" : "",
          resolvedColorClass,
          resolvedShadowClass,
          className,
        )}
        disabled={resolvedDisabled}
        size={resolvedSize}
        type={type}
        variant={resolvedVariant}
        onClick={handleClick}
        {...props}
      >
        {isLoading ? (
          <Loader2Icon className="mr-2 h-4 w-4 animate-spin" />
        ) : null}
        {startContent}
        {isIconOnly ? null : children}
        {isIconOnly ? children : null}
        {endContent}
      </BaseButton>
    );
  },
);

Button.displayName = "HeroBridgeButton";
