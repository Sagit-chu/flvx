import type { AnnouncementData } from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface AnnouncementModalProps {
  announcement: AnnouncementData;
  isOpen: boolean;
  onClose: () => void;
  onDontShowAgain: () => void;
}

export const AnnouncementModal = ({
  announcement,
  isOpen,
  onClose,
  onDontShowAgain,
}: AnnouncementModalProps) => {
  return (
    <Modal isOpen={isOpen} onOpenChange={(open) => !open && onClose()} size="2xl">
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">平台公告</ModalHeader>
        <ModalBody>
          <div className="prose prose-sm dark:prose-invert max-w-none max-h-[60vh] overflow-y-auto">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {announcement.content}
            </ReactMarkdown>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onDontShowAgain}>
            不再提示
          </Button>
          <Button color="primary" onPress={onClose}>
            关闭
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
