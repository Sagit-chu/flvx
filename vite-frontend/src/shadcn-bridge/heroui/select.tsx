import * as React from "react";
import { ChevronDownIcon } from "lucide-react";

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
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const listboxRef = React.useRef<HTMLDivElement | null>(null);
  const [isExpanded, setIsExpanded] = React.useState(false);
  const selected = React.useMemo(() => toSet(selectedKeys), [selectedKeys]);
  const disabled = React.useMemo(() => toSet(disabledKeys), [disabledKeys]);

  React.useEffect(() => {
    if (!isExpanded) {
      return;
    }

    const handlePointerDown = (event: MouseEvent | TouchEvent) => {
      const container = containerRef.current;
      const listbox = listboxRef.current;

      if (!container) {
        return;
      }

      const target = event.target;

      if (!(target instanceof Node)) {
        return;
      }

      if (container.contains(target) || listbox?.contains(target)) {
        return;
      }

      setIsExpanded(false);
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsExpanded(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("touchstart", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("touchstart", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [isExpanded]);

  React.useEffect(() => {
    if (isDisabled) {
      setIsExpanded(false);
    }
  }, [isDisabled]);

  const selectedArray = Array.from(selected);
  const singleValue = selectedArray[0] ?? "";
  const selectedLabels = options
    .filter((option) => selected.has(option.key))
    .map((option) => option.label);
  const resolvedSelectedValues =
    selectedLabels.length > 0 ? selectedLabels : selectedArray;
  const selectedFullText = resolvedSelectedValues.join("、");
  const selectedText =
    selectedArray.length > 0 ? selectedFullText : (placeholder ?? "请选择");

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

  const renderMultipleListbox = () => {
    if (!isExpanded) {
      return null;
    }

    return (
      <div
        className="absolute left-0 top-full z-50 mt-1 max-h-56 w-full space-y-1 overflow-y-auto rounded-md border border-divider bg-background p-2 shadow-md"
        id={`${generatedId}-listbox`}
        ref={listboxRef}
        role="listbox"
      >
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
    );
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
          ref={containerRef}
          className={cn(
            "relative w-full",
            className,
          )}
        >
          <button
            aria-controls={`${generatedId}-listbox`}
            aria-expanded={isExpanded}
            aria-haspopup="listbox"
            className={cn(
              "flex w-full min-w-0 items-center gap-2 overflow-hidden rounded-md border border-input bg-background px-3 py-2 text-left shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              isDisabled ? "cursor-not-allowed opacity-60" : "",
              classNames?.trigger,
            )}
            disabled={isDisabled}
            id={generatedId}
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
          >
            <span
              className={cn(
                "block min-w-0 flex-1 truncate",
                textSizeClass(size),
                selectedArray.length > 0
                  ? "text-foreground"
                  : "text-default-500",
              )}
              title={selectedArray.length > 0 ? selectedFullText : undefined}
            >
              {selectedText}
            </span>
            <ChevronDownIcon
              className={cn(
                "h-4 w-4 flex-shrink-0 text-default-500 transition-transform",
                isExpanded ? "rotate-180" : "rotate-0",
              )}
            />
          </button>
          {renderMultipleListbox()}
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
