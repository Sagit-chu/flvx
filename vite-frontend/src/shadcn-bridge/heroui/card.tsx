import * as React from "react";

import {
  Card as BaseCard,
  CardContent,
  CardHeader as BaseCardHeader,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function Card({ className, ...props }: React.ComponentProps<"div">) {
  return <BaseCard className={className} {...props} />;
}

export function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <BaseCardHeader className={cn("p-4 md:p-6", className)} {...props} />;
}

export function CardBody({ className, ...props }: React.ComponentProps<"div">) {
  return <CardContent className={cn("p-4 md:p-6", className)} {...props} />;
}
