import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { PageEmptyState } from "@/components/page-state";

interface FlowChartPoint {
  time: string;
  flow: number;
}

interface FlowChartCardProps {
  statisticsFlowsCount: number;
  chartData: FlowChartPoint[];
  formatFlow: (value: number, unit?: string) => string;
}

export const FlowChartCard = ({
  statisticsFlowsCount,
  chartData,
  formatFlow,
}: FlowChartCardProps) => {
  return (
    <Card className="mb-6 lg:mb-8 border border-gray-200 dark:border-default-200 shadow-md">
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <svg
            aria-hidden="true"
            className="w-5 h-5 text-primary"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path d="M2 10a8 8 0 018-8v8h8a8 8 0 11-16 0z" />
            <path d="M12 2.252A8.014 8.014 0 0117.748 8H12V2.252z" />
          </svg>
          <h2 className="text-lg lg:text-xl font-semibold text-foreground">
            24小时流量统计
          </h2>
        </div>
      </CardHeader>
      <CardBody className="pt-0">
        {statisticsFlowsCount === 0 ? (
          <PageEmptyState className="h-48" message="暂无流量统计数据" />
        ) : (
          <div className="h-64 lg:h-80 w-full">
            <ResponsiveContainer height="100%" width="100%">
              <LineChart data={chartData}>
                <CartesianGrid className="opacity-30" strokeDasharray="3 3" />
                <XAxis
                  axisLine={{ stroke: "#e5e7eb", strokeWidth: 1 }}
                  dataKey="time"
                  tick={{ fontSize: 12 }}
                  tickLine={false}
                />
                <YAxis
                  axisLine={{ stroke: "#e5e7eb", strokeWidth: 1 }}
                  tick={{ fontSize: 12 }}
                  tickFormatter={(value: number | string) => {
                    const v = typeof value === "number" ? value : Number(value);

                    if (!Number.isFinite(v)) return String(value);
                    if (v === 0) return "0";
                    if (v < 1024) return `${v}B`;
                    if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)}K`;
                    if (v < 1024 * 1024 * 1024)
                      return `${(v / (1024 * 1024)).toFixed(1)}M`;

                    return `${(v / (1024 * 1024 * 1024)).toFixed(1)}G`;
                  }}
                  tickLine={false}
                />
                <Tooltip
                  content={({
                    active,
                    payload,
                    label,
                  }: {
                    active?: boolean;
                    payload?: Array<{ value?: number | string }>;
                    label?: string | number;
                  }) => {
                    if (active && payload && payload.length) {
                      const firstValue = payload[0]?.value;
                      const numericValue =
                        typeof firstValue === "number"
                          ? firstValue
                          : Number(firstValue);
                      const flowValue = Number.isFinite(numericValue)
                        ? numericValue
                        : 0;

                      return (
                        <div className="bg-white dark:bg-default-100 border border-default-200 rounded-lg shadow-lg p-3">
                          <p className="font-medium text-foreground">{`时间: ${label ?? ""}`}</p>
                          <p className="text-primary">{`流量: ${formatFlow(flowValue)}`}</p>
                        </div>
                      );
                    }

                    return null;
                  }}
                />
                <Line
                  activeDot={{
                    r: 4,
                    stroke: "#8b5cf6",
                    strokeWidth: 2,
                    fill: "#fff",
                  }}
                  dataKey="flow"
                  dot={false}
                  stroke="#8b5cf6"
                  strokeWidth={3}
                  type="monotone"
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardBody>
    </Card>
  );
};
