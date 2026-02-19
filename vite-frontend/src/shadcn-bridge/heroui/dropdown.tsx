import * as React from "react";

import {
  DropdownMenu as BaseDropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem as BaseDropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger as BaseDropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type Placement = "bottom-start" | "bottom-end" | "top-start" | "top-end";

interface DropdownContextValue {
  align: "start" | "end";
}

const DropdownContext = React.createContext<DropdownContextValue>({
  align: "start",
});

export interface DropdownProps {
  children: React.ReactNode;
  placement?: Placement;
}

export function Dropdown({
  children,
  placement = "bottom-start",
}: DropdownProps) {
  const align = placement.endsWith("end") ? "end" : "start";

  return (
    <DropdownContext.Provider value={{ align }}>
      <BaseDropdownMenu>{children}</BaseDropdownMenu>
    </DropdownContext.Provider>
  );
}

export function DropdownTrigger({ children }: { children: React.ReactNode }) {
  return <BaseDropdownMenuTrigger asChild>{children}</BaseDropdownMenuTrigger>;
}

export interface DropdownMenuProps {
  "aria-label"?: string;
  children: React.ReactNode;
}

export function DropdownMenu({ children }: DropdownMenuProps) {
  const { align } = React.useContext(DropdownContext);

  return <DropdownMenuContent align={align}>{children}</DropdownMenuContent>;
}

export interface DropdownItemProps {
  children: React.ReactNode;
  className?: string;
  color?: "default" | "danger";
  onPress?: () => void;
  startContent?: React.ReactNode;
}

export function DropdownItem({
  children,
  className,
  color = "default",
  onPress,
  startContent,
}: DropdownItemProps) {
  return (
    <BaseDropdownMenuItem
      className={cn(
        color === "danger" ? "text-danger focus:text-danger" : "",
        className,
      )}
      onSelect={(event) => {
        event.preventDefault();
        onPress?.();
      }}
    >
      {startContent}
      {children}
    </BaseDropdownMenuItem>
  );
}

export { DropdownMenuLabel, DropdownMenuSeparator };
