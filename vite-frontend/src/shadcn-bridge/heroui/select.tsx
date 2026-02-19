import * as React from "react";

import { cn } from "@/lib/utils";

import { FieldContainer, extractText, type FieldMetaProps } from "./shared";

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
  const options = React.useMemo(() => getOptions(children, items), [children, items]);
  const selected = React.useMemo(() => toSet(selectedKeys), [selectedKeys]);
  const disabled = React.useMemo(() => toSet(disabledKeys), [disabledKeys]);

  const selectedArray = Array.from(selected);
  const singleValue = selectedArray[0] ?? "";

  const handleChange = (event: React.ChangeEvent<HTMLSelectElement>) => {
    onChange?.(event);

    if (!onSelectionChange) {
      return;
    }

    if (selectionMode === "multiple") {
      const values = Array.from(event.target.selectedOptions).map((option) => option.value);

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
      <select
        className={cn(
          "w-full rounded-md border border-input bg-background px-3 py-2 shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          sizeClass(size),
          selectionMode === "multiple" ? "min-h-32" : "",
          classNames?.trigger,
          className,
        )}
        disabled={isDisabled}
        id={generatedId}
        multiple={selectionMode === "multiple"}
        required={isRequired}
        value={selectionMode === "multiple" ? selectedArray : singleValue}
        onClick={onClick}
        onChange={handleChange}
      >
        {selectionMode === "single" ? (
          <option value="">{placeholder ?? "请选择"}</option>
        ) : null}
        {options.map((option) => (
          <option key={option.key} disabled={disabled.has(option.key)} value={option.key}>
            {option.label}
          </option>
        ))}
      </select>
    </FieldContainer>
  );
}
