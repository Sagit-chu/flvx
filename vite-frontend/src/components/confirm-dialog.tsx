import type { ReactNode } from "react";

import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";

type ConfirmDialogProps = {
  cancelText?: string;
  children?: ReactNode;
  confirmText?: string;
  description?: string;
  isLoading?: boolean;
  isOpen: boolean;
  title: string;
  tone?: "danger" | "primary" | "warning";
  onConfirm: () => void | Promise<void>;
  onOpenChange: (open: boolean) => void;
};

export function ConfirmDialog({
  cancelText = "取消",
  children,
  confirmText = "确认",
  description,
  isLoading = false,
  isOpen,
  title,
  tone = "danger",
  onConfirm,
  onOpenChange,
}: ConfirmDialogProps) {
  return (
    <Modal
      backdrop="blur"
      classNames={{
        base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-[var(--radius-panel)] overflow-hidden",
      }}
      isOpen={isOpen}
      placement="center"
      size="md"
      onOpenChange={onOpenChange}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <h2 className="text-lg font-semibold text-foreground">{title}</h2>
            </ModalHeader>
            <ModalBody>
              {description ? (
                <p className="text-sm leading-6 text-default-600">
                  {description}
                </p>
              ) : null}
              {children}
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                {cancelText}
              </Button>
              <Button color={tone} isLoading={isLoading} onPress={onConfirm}>
                {confirmText}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
