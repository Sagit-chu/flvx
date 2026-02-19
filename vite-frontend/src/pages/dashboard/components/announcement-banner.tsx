import type { AnnouncementData } from "@/api";

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
        <div className="flex items-start gap-3">
          <div className="p-2 bg-blue-100 dark:bg-blue-500/20 rounded-lg flex-shrink-0">
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
          <div className="flex-1 min-w-0">
            <h3 className="text-sm lg:text-base font-semibold text-blue-900 dark:text-blue-100 mb-1">
              公告
            </h3>
            <p className="text-xs lg:text-sm text-blue-800 dark:text-blue-200 whitespace-pre-wrap break-words">
              {announcement.content}
            </p>
          </div>
        </div>
      </CardBody>
    </Card>
  );
};
