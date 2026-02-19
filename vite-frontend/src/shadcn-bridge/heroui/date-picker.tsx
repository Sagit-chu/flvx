import * as React from "react";

import { FieldContainer, type FieldMetaProps } from "./shared";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

interface CalendarDateLike {
  day: number;
  month: number;
  year: number;
}

export interface DatePickerProps extends FieldMetaProps {
  className?: string;
  isDisabled?: boolean;
  isRequired?: boolean;
  onChange?: (value: CalendarDateLike | null) => void;
  showMonthAndYearPickers?: boolean;
  value?: CalendarDateLike | null;
}

function formatDateValue(value: CalendarDateLike | null | undefined) {
  if (!value) {
    return "";
  }
  const month = String(value.month).padStart(2, "0");
  const day = String(value.day).padStart(2, "0");

  return `${value.year}-${month}-${day}`;
}

export function DatePicker({
  className,
  description,
  errorMessage,
  isDisabled,
  isInvalid,
  isRequired,
  label,
  onChange,
  value,
}: DatePickerProps) {
  const id = React.useId();

  return (
    <FieldContainer
      description={description}
      errorMessage={errorMessage}
      id={id}
      isInvalid={isInvalid}
      isRequired={isRequired}
      label={label}
    >
      <Input
        aria-invalid={isInvalid}
        className={cn(className)}
        disabled={isDisabled}
        id={id}
        required={isRequired}
        type="date"
        value={formatDateValue(value)}
        onChange={(event) => {
          if (!onChange) {
            return;
          }

          if (!event.target.value) {
            onChange(null);

            return;
          }

          const [yearText, monthText, dayText] = event.target.value.split("-");
          const year = Number(yearText);
          const month = Number(monthText);
          const day = Number(dayText);

          if (
            !Number.isFinite(year) ||
            !Number.isFinite(month) ||
            !Number.isFinite(day)
          ) {
            onChange(null);

            return;
          }

          onChange({ day, month, year });
        }}
      />
    </FieldContainer>
  );
}
