import * as React from "react";

import { Dialog, DialogContent as BaseDialogContent } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

interface DisclosureOptions {
  isOpen?: boolean;
}

type OpenChangeHandler = (open?: boolean) => void;

export function useDisclosure(options: DisclosureOptions = {}) {
  const [isOpen, setIsOpen] = React.useState(Boolean(options.isOpen));

  const onOpen = React.useCallback(() => {
    setIsOpen(true);
  }, []);

  const onClose = React.useCallback(() => {
    setIsOpen(false);
  }, []);

  const onOpenChange = React.useCallback<OpenChangeHandler>((open) => {
    setIsOpen((prev) => (typeof open === "boolean" ? open : !prev));
  }, []);

  return {
    isOpen,
    onClose,
    onOpen,
    onOpenChange,
  };
}

interface ModalContextValue {
  classNames?: Record<string, string>;
  onClose: () => void;
}

const ModalContext = React.createContext<ModalContextValue | null>(null);

function useModalContext() {
  return React.useContext(ModalContext);
}

type ModalSize = "sm" | "md" | "lg" | "xl" | "2xl" | "4xl" | "full";

function mapSize(size: ModalSize | undefined) {
  if (size === "sm") {
    return "max-w-md";
  }
  if (size === "lg") {
    return "max-w-2xl";
  }
  if (size === "xl") {
    return "max-w-3xl";
  }
  if (size === "2xl") {
    return "max-w-5xl";
  }
  if (size === "4xl") {
    return "max-w-6xl";
  }
  if (size === "full") {
    return "w-[95vw] max-w-[95vw] h-[95vh] max-h-[95vh]";
  }

  return "max-w-lg";
}

export interface ModalProps {
  backdrop?: "blur" | "opaque" | "transparent";
  children: React.ReactNode;
  className?: string;
  classNames?: Record<string, string>;
  isDismissable?: boolean;
  isOpen?: boolean;
  onClose?: () => void;
  onOpenChange?: (open: boolean) => void;
  placement?: "center" | "top" | "bottom";
  scrollBehavior?: "inside" | "outside";
  size?: ModalSize;
}

export function Modal({
  children,
  classNames,
  isOpen = false,
  onClose,
  onOpenChange,
}: ModalProps) {
  const handleOpenChange = (open: boolean) => {
    onOpenChange?.(open);
    if (!open) {
      onClose?.();
    }
  };

  const contextValue = {
    classNames,
    onClose: () => {
      handleOpenChange(false);
    },
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <ModalContext.Provider value={contextValue}>{children}</ModalContext.Provider>
    </Dialog>
  );
}

interface ModalContentProps extends Omit<React.ComponentProps<"div">, "children"> {
  children: React.ReactNode | ((onClose: () => void) => React.ReactNode);
  scrollBehavior?: "inside" | "outside";
  size?: ModalSize;
}

export function ModalContent({
  children,
  className,
  scrollBehavior,
  size,
  ...props
}: ModalContentProps) {
  const context = useModalContext();
  const renderedChildren =
    typeof children === "function" ? children(() => context?.onClose()) : children;

  return (
    <BaseDialogContent
      className={cn(
        mapSize(size),
        context?.classNames?.base,
        scrollBehavior === "outside" ? "max-h-[90vh] overflow-y-auto" : "",
        className,
      )}
      showCloseButton={false}
      {...props}
    >
      {renderedChildren}
    </BaseDialogContent>
  );
}

export function ModalHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("text-lg font-semibold", className)} {...props} />;
}

export function ModalBody({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("space-y-4", className)} {...props} />;
}

export function ModalFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("mt-4 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    />
  );
}
