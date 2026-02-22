import * as React from "react";

import { FieldContainer, type FieldMetaProps } from "./shared";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

interface CalendarDateLike {
  day: number;
  month: number;
  year: number;
}

function isValidCalendarDate(year: number, month: number, day: number) {
  if (
    !Number.isInteger(year) ||
    !Number.isInteger(month) ||
    !Number.isInteger(day)
  ) {
    return false;
  }
  if (month < 1 || month > 12 || day < 1 || day > 31) {
    return false;
  }
  const candidate = new Date(year, month - 1, day);

  return (
    candidate.getFullYear() === year &&
    candidate.getMonth() === month - 1 &&
    candidate.getDate() === day
  );
}

function parseDateText(value: string) {
  const trimmed = value.trim();

  if (!trimmed) {
    return null;
  }

  const matched = trimmed.match(/^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})$/);

  if (!matched) {
    return null;
  }

  const year = Number(matched[1]);
  const month = Number(matched[2]);
  const day = Number(matched[3]);

  if (!isValidCalendarDate(year, month, day)) {
    return null;
  }

  return { day, month, year };
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
  showMonthAndYearPickers,
  value,
}: DatePickerProps) {
  const id = React.useId();
  const formattedValue = React.useMemo(() => formatDateValue(value), [value]);
  const [textValue, setTextValue] = React.useState(formattedValue);

  React.useEffect(() => {
    setTextValue(formattedValue);
  }, [formattedValue]);

  const shouldUseTextInput = Boolean(showMonthAndYearPickers);

  const notifyNativeDateChange = (rawValue: string) => {
    if (!onChange) {
      return;
    }

    if (!rawValue) {
      onChange(null);

      return;
    }

    const [yearText, monthText, dayText] = rawValue.split("-");
    const year = Number(yearText);
    const month = Number(monthText);
    const day = Number(dayText);

    if (!isValidCalendarDate(year, month, day)) {
      onChange(null);

      return;
    }

    onChange({ day, month, year });
  };

  const notifyTextDateChange = (rawValue: string) => {
    if (!onChange) {
      return;
    }

    if (!rawValue.trim()) {
      onChange(null);

      return;
    }

    const parsed = parseDateText(rawValue);

    if (parsed) {
      onChange(parsed);
    }
  };

  const commitTextInput = () => {
    const parsed = parseDateText(textValue);

    if (parsed) {
      setTextValue(formatDateValue(parsed));
      onChange?.(parsed);

      return;
    }

    if (!textValue.trim()) {
      onChange?.(null);

      return;
    }

    setTextValue(formattedValue);
  };

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
        inputMode={shouldUseTextInput ? "numeric" : undefined}
        placeholder={shouldUseTextInput ? "YYYY-MM-DD" : undefined}
        required={isRequired}
        type={shouldUseTextInput ? "text" : "date"}
        value={shouldUseTextInput ? textValue : formattedValue}
        onBlur={shouldUseTextInput ? commitTextInput : undefined}
        onChange={(event) => {
          const nextValue = event.target.value;

          if (shouldUseTextInput) {
            setTextValue(nextValue);
            notifyTextDateChange(nextValue);

            return;
          }

          notifyNativeDateChange(nextValue);
        }}
        onKeyDown={
          shouldUseTextInput
            ? (event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  commitTextInput();
                }
              }
            : undefined
        }
      />
    </FieldContainer>
  );
}
