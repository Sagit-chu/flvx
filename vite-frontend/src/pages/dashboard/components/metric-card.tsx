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
    <Card className="h-48 flex flex-col justify-between overflow-hidden">
      <CardBody className="!p-4 md:!p-6 h-full flex flex-col justify-between overflow-visible">
        <div className="flex flex-col space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium text-default-600 truncate">
              {title}
            </p>
            <div
              className={`p-2 rounded-xl flex-shrink-0 ${iconClassName}`}
            >
              {icon}
            </div>
          </div>
          <p className="text-2xl lg:text-3xl font-bold text-foreground truncate mt-2">
            {value}
          </p>
        </div>
        <div className="mt-auto pt-4">
          {bottomContent}
        </div>
      </CardBody>
    </Card>
  );
};
