import { motion, AnimatePresence } from "framer-motion";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import { SearchIcon, XIcon } from "@/components/icons";

interface SearchBarProps {
  isVisible: boolean;
  value: string;
  placeholder?: string;
  onOpen: () => void;
  onClose: () => void;
  onChange: (value: string) => void;
}

export function SearchBar({
  isVisible,
  value,
  placeholder = "搜索",
  onOpen,
  onClose,
  onChange,
}: SearchBarProps) {
  return (
    // Fixed h-8 so the container never changes height — eliminates the vertical jitter
    <div className="flex items-center gap-2 h-8 overflow-hidden">
      <AnimatePresence initial={false} mode="wait">
        {!isVisible ? (
          <motion.div
            key="search-btn"
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0 }}
            initial={{ opacity: 0 }}
            transition={{ duration: 0.12 }}
          >
            <Button
              isIconOnly
              aria-label="搜索"
              className="bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/45 h-8 w-8 min-w-8"
              color="default"
              size="sm"
              title="搜索"
              variant="flat"
              onPress={onOpen}
            >
              <SearchIcon className="w-4 h-4" />
            </Button>
          </motion.div>
        ) : (
          <motion.div
            key="search-input"
            animate={{ opacity: 1, x: 0 }}
            className="flex w-full items-center gap-2"
            exit={{ opacity: 0, x: -8 }}
            initial={{ opacity: 0, x: -16 }}
            transition={{ duration: 0.18, ease: [0.25, 0.46, 0.45, 0.94] }}
          >
            <Input
              classNames={{
                base: "bg-default-100",
                input:
                  "bg-transparent text-sm focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none",
                inputWrapper: "bg-default-100 border-0 shadow-none h-8 min-h-8",
              }}
              placeholder={placeholder}
              value={value}
              onChange={(e) => onChange(e.target.value)}
            />
            <Button
              isIconOnly
              aria-label="关闭搜索"
              className="text-default-600 shrink-0"
              color="default"
              size="sm"
              title="关闭搜索"
              variant="light"
              onPress={() => {
                onClose();
                onChange("");
              }}
            >
              <XIcon className="w-4 h-4" />
            </Button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
