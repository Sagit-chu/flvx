import type { AnnouncementData } from "@/api";
import ReactMarkdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

import { Card, CardBody } from "@/shadcn-bridge/heroui/card";

interface AnnouncementBannerProps {
  announcement: AnnouncementData;
}

export const AnnouncementBanner = ({
  announcement,
}: AnnouncementBannerProps) => {
  if (!announcement.content) {
    return null;
  }

  return (
    <Card className="mb-4 lg:mb-6 border border-blue-200 dark:border-blue-500/30 bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-500/10 dark:to-purple-500/10">
      <CardBody className="p-4">
        <div className="flex items-start justify-start gap-3.5">
          <div className="w-10 h-10 bg-blue-100 dark:bg-blue-500/20 rounded-lg flex-shrink-0 flex items-center justify-center mt-0.5">
            <svg
              aria-hidden="true"
              className="w-5 h-5 text-blue-600 dark:text-blue-400"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path
                clipRule="evenodd"
                d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
                fillRule="evenodd"
              />
            </svg>
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <h3 className="text-base font-semibold leading-none text-blue-900 dark:text-blue-100 mb-1.5">
              公告
            </h3>
            <div className="text-sm text-blue-800 dark:text-blue-200 break-words leading-relaxed">
              <ReactMarkdown
                rehypePlugins={[rehypeSanitize]}
                remarkPlugins={[remarkGfm]}
                components={{
                  p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
                  a: ({ children, href }) => (
                    <a
                      className="underline decoration-blue-500/70 underline-offset-2 hover:text-blue-700 dark:hover:text-blue-100"
                      href={href}
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      {children}
                    </a>
                  ),
                  ul: ({ children }) => (
                    <ul className="list-disc pl-5 space-y-1 mb-2 last:mb-0">{children}</ul>
                  ),
                  ol: ({ children }) => (
                    <ol className="list-decimal pl-5 space-y-1 mb-2 last:mb-0">{children}</ol>
                  ),
                  code: ({ children }) => (
                    <code className="rounded bg-blue-100/80 dark:bg-blue-900/40 px-1 py-0.5 text-[0.92em]">
                      {children}
                    </code>
                  ),
                  pre: ({ children }) => (
                    <pre className="mb-2 overflow-x-auto rounded-md bg-blue-100/70 dark:bg-blue-900/40 p-2.5 text-xs leading-relaxed">
                      {children}
                    </pre>
                  ),
                  blockquote: ({ children }) => (
                    <blockquote className="mb-2 border-l-2 border-blue-300/80 dark:border-blue-500/60 pl-3 italic">
                      {children}
                    </blockquote>
                  ),
                }}
              >
                {announcement.content}
              </ReactMarkdown>
            </div>
          </div>
        </div>
      </CardBody>
    </Card>
  );
};
