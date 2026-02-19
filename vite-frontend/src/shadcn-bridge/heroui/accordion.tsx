import * as React from "react";

import {
  Accordion as BaseAccordion,
  AccordionContent,
  AccordionItem as BaseAccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { cn } from "@/lib/utils";

export interface AccordionProps extends Omit<React.ComponentProps<"div">, "children"> {
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

export function Accordion({ children, className }: AccordionProps) {
  return (
    <BaseAccordion className={cn("w-full", className)} type="multiple">
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
