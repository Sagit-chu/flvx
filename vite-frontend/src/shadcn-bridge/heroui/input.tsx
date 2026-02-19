import * as React from "react";

import { FieldContainer, type FieldMetaProps } from "./shared";

import { Input as BaseInput } from "@/components/ui/input";
import { Textarea as BaseTextarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

type ClassNameMap = {
  base?: string;
  description?: string;
  errorMessage?: string;
  input?: string;
  inputWrapper?: string;
  label?: string;
};

export interface InputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "size">,
    FieldMetaProps {
  classNames?: ClassNameMap;
  endContent?: React.ReactNode;
  isDisabled?: boolean;
  size?: "sm" | "md" | "lg";
  startContent?: React.ReactNode;
  variant?: "flat" | "bordered" | "faded" | "underlined";
}

export interface TextareaProps
  extends Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, "size">,
    FieldMetaProps {
  classNames?: ClassNameMap;
  isDisabled?: boolean;
  maxRows?: number;
  minRows?: number;
  size?: "sm" | "md" | "lg";
  variant?: "flat" | "bordered" | "faded" | "underlined";
}

function inputSizeClass(size: "sm" | "md" | "lg" | undefined) {
  if (size === "sm") {
    return "h-8 text-xs";
  }
  if (size === "lg") {
    return "h-10 text-base";
  }

  return "h-9 text-sm";
}

export function Input({
  className,
  classNames,
  description,
  endContent,
  errorMessage,
  id,
  isDisabled,
  isInvalid,
  isRequired,
  label,
  size,
  startContent,
  variant,
  ...props
}: InputProps) {
  const generatedId = React.useId();
  const resolvedId = id ?? generatedId;

  return (
    <FieldContainer
      className={classNames?.base}
      description={description}
      errorMessage={errorMessage}
      id={resolvedId}
      isInvalid={isInvalid}
      isRequired={isRequired}
      label={label}
    >
      <div
        className={cn(
          "relative flex items-center rounded-md",
          variant === "bordered" ? "border border-input" : "",
          classNames?.inputWrapper,
        )}
      >
        {startContent ? (
          <div className="pl-3 text-default-500">{startContent}</div>
        ) : null}
        <BaseInput
          aria-invalid={isInvalid}
          className={cn(
            inputSizeClass(size),
            variant === "bordered" ? "border-0 shadow-none" : "",
            startContent ? "pl-2" : "",
            endContent ? "pr-2" : "",
            classNames?.input,
            className,
          )}
          disabled={isDisabled}
          id={resolvedId}
          required={isRequired}
          {...props}
        />
        {endContent ? (
          <div className="pr-3 text-default-500">{endContent}</div>
        ) : null}
      </div>
    </FieldContainer>
  );
}

export function Textarea({
  className,
  classNames,
  description,
  errorMessage,
  id,
  isDisabled,
  isInvalid,
  isRequired,
  label,
  maxRows,
  minRows,
  size,
  variant,
  ...props
}: TextareaProps) {
  const generatedId = React.useId();
  const resolvedId = id ?? generatedId;
  const rows = props.rows ?? minRows ?? 3;

  return (
    <FieldContainer
      className={classNames?.base}
      description={description}
      errorMessage={errorMessage}
      id={resolvedId}
      isInvalid={isInvalid}
      isRequired={isRequired}
      label={label}
    >
      <BaseTextarea
        aria-invalid={isInvalid}
        className={cn(
          variant === "bordered" ? "border border-input" : "",
          size === "sm" ? "text-xs" : "",
          size === "lg" ? "text-base" : "",
          maxRows ? "max-h-[40vh]" : "",
          classNames?.input,
          className,
        )}
        disabled={isDisabled}
        id={resolvedId}
        required={isRequired}
        rows={rows}
        {...props}
      />
    </FieldContainer>
  );
}
