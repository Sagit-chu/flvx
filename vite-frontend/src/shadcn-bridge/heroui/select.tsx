import * as React from "react";

import { FieldContainer, extractText, type FieldMetaProps } from "./shared";

import { Checkbox as BaseCheckbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";

type SelectionMode = "single" | "multiple";

type SelectionValue = Iterable<React.Key> | Set<React.Key> | Array<React.Key>;

interface OptionItem {
  disabled?: boolean;
  key: string;
  label: string;
}

interface ClassNameMap {
  base?: string;
  trigger?: string;
  [key: string]: string | undefined;
}

export interface SelectProps<T = unknown> extends FieldMetaProps {
  children?: React.ReactNode | ((item: T) => React.ReactNode);
  className?: string;
  classNames?: ClassNameMap;
  disabledKeys?: SelectionValue;
  isDisabled?: boolean;
  items?: Iterable<T>;
  onChange?: (event: React.ChangeEvent<HTMLSelectElement>) => void;
  onClick?: (event: React.MouseEvent<HTMLSelectElement>) => void;
  onSelectionChange?: (keys: Set<React.Key>) => void;
  placeholder?: string;
  selectedKeys?: SelectionValue;
  selectionMode?: SelectionMode;
  size?: "sm" | "md" | "lg";
  variant?: string;
}

export interface SelectItemProps {
  children?: React.ReactNode;
  description?: React.ReactNode;
  textValue?: string;
}

export function SelectItem(_props: SelectItemProps) {
  return null;
}

SelectItem.displayName = "HeroSelectItem";

function toSet(value?: SelectionValue) {
  if (!value) {
    return new Set<string>();
  }

  return new Set(Array.from(value).map((item) => String(item)));
}

function flattenOptionsFromNode(node: React.ReactNode, options: OptionItem[]) {
  React.Children.forEach(node, (child, index) => {
    if (child === null || child === undefined || typeof child === "boolean") {
      return;
    }
    if (Array.isArray(child)) {
      flattenOptionsFromNode(child, options);

      return;
    }
    if (React.isValidElement(child)) {
      if (child.type === React.Fragment) {
        flattenOptionsFromNode(child.props.children, options);

        return;
      }

      if (child.type === SelectItem) {
        const key = child.key ? String(child.key) : String(index);
        const props = child.props as SelectItemProps;

        options.push({
          key,
          label: props.textValue ?? extractText(props.children) ?? key,
        });

        return;
      }
    }
  });
}

function getOptions<T>(
  children: React.ReactNode | ((item: T) => React.ReactNode) | undefined,
  items: Iterable<T> | undefined,
) {
  const options: OptionItem[] = [];

  if (typeof children === "function" && items) {
    Array.from(items).forEach((item, index) => {
      const rendered = children(item);

      if (React.isValidElement(rendered) && rendered.type === SelectItem) {
        const key = rendered.key ? String(rendered.key) : String(index);
        const props = rendered.props as SelectItemProps;

        options.push({
          key,
          label: props.textValue ?? extractText(props.children) ?? key,
        });
      }
    });

    return options;
  }

  if (typeof children !== "function") {
    flattenOptionsFromNode(children, options);
  }

  return options;
}

function sizeClass(size: SelectProps["size"]) {
  if (size === "sm") {
    return "h-8 text-xs";
  }
  if (size === "lg") {
    return "h-10 text-base";
  }

  return "h-9 text-sm";
}

function textSizeClass(size: SelectProps["size"]) {
  if (size === "sm") {
    return "text-xs";
  }
  if (size === "lg") {
    return "text-base";
  }

  return "text-sm";
}

export function Select<T>({
  children,
  className,
  classNames,
  description,
  disabledKeys,
  errorMessage,
  isDisabled,
  isInvalid,
  isRequired,
  items,
  label,
  onChange,
  onClick,
  onSelectionChange,
  placeholder,
  selectedKeys,
  selectionMode = "single",
  size,
}: SelectProps<T>) {
  const generatedId = React.useId();
  const options = React.useMemo(
    () => getOptions(children, items),
    [children, items],
  );
  const selected = React.useMemo(() => toSet(selectedKeys), [selectedKeys]);
  const disabled = React.useMemo(() => toSet(disabledKeys), [disabledKeys]);

  const selectedArray = Array.from(selected);
  const singleValue = selectedArray[0] ?? "";
  const selectedText =
    selectedArray.length > 0
      ? options
          .filter((option) => selected.has(option.key))
          .map((option) => option.label)
          .join("、")
      : (placeholder ?? "请选择");

  const updateMultipleSelection = (key: string, checked?: boolean) => {
    if (isDisabled || disabled.has(key)) {
      return;
    }

    const next = new Set(selected);
    const shouldSelect =
      typeof checked === "boolean" ? checked : !next.has(key);

    if (shouldSelect) {
      next.add(key);
    } else {
      next.delete(key);
    }

    onSelectionChange?.(next);
  };

  const handleChange = (event: React.ChangeEvent<HTMLSelectElement>) => {
    onChange?.(event);

    if (!onSelectionChange) {
      return;
    }

    if (selectionMode === "multiple") {
      const values = Array.from(event.target.selectedOptions).map(
        (option) => option.value,
      );

      onSelectionChange(new Set(values));

      return;
    }

    if (!event.target.value) {
      onSelectionChange(new Set());

      return;
    }

    onSelectionChange(new Set([event.target.value]));
  };

  return (
    <FieldContainer
      className={classNames?.base}
      description={description}
      errorMessage={errorMessage}
      id={generatedId}
      isInvalid={isInvalid}
      isRequired={isRequired}
      label={label}
    >
      {selectionMode === "multiple" ? (
        <div
          className={cn(
            "w-full rounded-md border border-input bg-background shadow-sm",
            isDisabled ? "cursor-not-allowed opacity-60" : "",
            classNames?.trigger,
            className,
          )}
          id={generatedId}
        >
          <div
            className={cn(
              "border-b border-divider px-3 py-2 text-default-500",
              textSizeClass(size),
              selectedArray.length > 0 ? "text-foreground" : "",
            )}
            title={selectedText}
          >
            <span className="block truncate">{selectedText}</span>
          </div>
          <div className="max-h-56 space-y-1 overflow-y-auto p-2">
            {options.length === 0 ? (
              <div
                className={cn(
                  "px-2 py-1 text-default-500",
                  textSizeClass(size),
                )}
              >
                暂无可选项
              </div>
            ) : (
              options.map((option) => {
                const optionDisabled = isDisabled || disabled.has(option.key);

                return (
                  <div
                    key={option.key}
                    className={cn(
                      "flex items-center gap-2 rounded-md px-2 py-1.5",
                      optionDisabled
                        ? "cursor-not-allowed opacity-60"
                        : "hover:bg-default-100",
                    )}
                  >
                    <BaseCheckbox
                      checked={selected.has(option.key)}
                      disabled={optionDisabled}
                      onCheckedChange={(value) =>
                        updateMultipleSelection(option.key, value === true)
                      }
                    />
                    <button
                      className={cn(
                        "min-w-0 flex-1 truncate text-left text-foreground",
                        textSizeClass(size),
                        optionDisabled
                          ? "cursor-not-allowed"
                          : "cursor-pointer",
                      )}
                      disabled={optionDisabled}
                      type="button"
                      onClick={() => updateMultipleSelection(option.key)}
                    >
                      {option.label}
                    </button>
                  </div>
                );
              })
            )}
          </div>
        </div>
      ) : (
        <select
          className={cn(
            "w-full rounded-md border border-input bg-background px-3 py-2 shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            sizeClass(size),
            classNames?.trigger,
            className,
          )}
          disabled={isDisabled}
          id={generatedId}
          required={isRequired}
          value={singleValue}
          onChange={handleChange}
          onClick={onClick}
        >
          <option value="">{placeholder ?? "请选择"}</option>
          {options.map((option) => (
            <option
              key={option.key}
              disabled={disabled.has(option.key)}
              value={option.key}
            >
              {option.label}
            </option>
          ))}
        </select>
      )}
    </FieldContainer>
  );
}
