import * as React from "react";

import {
  Accordion as BaseAccordion,
  AccordionContent,
  AccordionItem as BaseAccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { cn } from "@/lib/utils";

export interface AccordionProps
  extends Omit<React.ComponentProps<"div">, "children"> {
  children: React.ReactNode;
  variant?: "bordered" | "light" | "splitted";
}

export interface AccordionItemProps {
  "aria-label"?: string;
  children: React.ReactNode;
  className?: string;
  title: React.ReactNode;
  value?: string;
}

function variantClass(variant: AccordionProps["variant"]) {
  if (variant === "splitted") {
    return "[&>[data-slot=accordion-item]]:mb-3 [&>[data-slot=accordion-item]]:rounded-xl [&>[data-slot=accordion-item]]:border [&>[data-slot=accordion-item]]:border-divider [&>[data-slot=accordion-item]]:border-b-0 [&>[data-slot=accordion-item]]:bg-content1 [&>[data-slot=accordion-item]]:shadow-sm [&>[data-slot=accordion-item]]:overflow-hidden [&>[data-slot=accordion-item]:last-child]:mb-0";
  }
  if (variant === "bordered") {
    return "rounded-xl border border-divider bg-content1 overflow-hidden [&>[data-slot=accordion-item]]:border-b [&>[data-slot=accordion-item]:last-child]:border-b-0";
  }

  return "";
}

export function Accordion({
  children,
  className,
  variant = "light",
}: AccordionProps) {
  return (
    <BaseAccordion
      className={cn("w-full", variantClass(variant), className)}
      type="multiple"
    >
      {children}
    </BaseAccordion>
  );
}

export function AccordionItem({
  "aria-label": ariaLabel,
  children,
  className,
  title,
  value,
}: AccordionItemProps) {
  const generatedValue = React.useId();

  return (
    <BaseAccordionItem className={className} value={value ?? generatedValue}>
      <AccordionTrigger aria-label={ariaLabel}>{title}</AccordionTrigger>
      <AccordionContent>{children}</AccordionContent>
    </BaseAccordionItem>
  );
}
