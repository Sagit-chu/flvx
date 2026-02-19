import type { ReactNode } from "react";

import { Card, CardBody } from "@/shadcn-bridge/heroui/card";

interface MetricCardProps {
  title: string;
  value: string | number;
  iconClassName: string;
  icon: ReactNode;
  bottomContent?: ReactNode;
}

export const MetricCard = ({
  title,
  value,
  iconClassName,
  icon,
  bottomContent,
}: MetricCardProps) => {
  return (
    <Card className="border border-gray-200 dark:border-default-200 shadow-md hover:shadow-lg transition-shadow">
      <CardBody className="p-3 lg:p-4">
        <div className="flex flex-col space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs lg:text-sm text-default-600 truncate">
              {title}
            </p>
            <div
              className={`p-1.5 lg:p-2 rounded-lg flex-shrink-0 ${iconClassName}`}
            >
              {icon}
            </div>
          </div>
          <p className="text-base lg:text-xl font-bold text-foreground truncate">
            {value}
          </p>
          {bottomContent}
        </div>
      </CardBody>
    </Card>
  );
};
