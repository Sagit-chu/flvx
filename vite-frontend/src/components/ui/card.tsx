import * as React from "react";
import LiquidGlass from "liquid-glass-react";

import { cn } from "@/lib/utils";

function Card({ className, style, ...props }: React.ComponentProps<"div">) {
  return (
    <LiquidGlass
      className={cn(
        "rounded-2xl border border-white/80 dark:border-white/10 bg-white/60 dark:bg-zinc-900/60 backdrop-blur-3xl text-card-foreground shadow-[0_10px_30px_rgba(0,0,0,0.1)]",
        className,
      )}
      style={style}
      blurAmount={0.05}
      saturation={120}
      displacementScale={30}
      aberrationIntensity={0.5}
      cornerRadius={24}
      padding="0px"
    >
      <div
        className="w-full h-full flex flex-col"
        data-slot="card"
        {...props}
      />
    </LiquidGlass>
  );
}

function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("flex flex-col gap-1.5 p-6", className)}
      data-slot="card-header"
      {...props}
    />
  );
}

function CardTitle({
  className,
  children,
  ...props
}: React.ComponentProps<"h3">) {
  return (
    <h3
      className={cn(
        "text-lg font-semibold leading-none tracking-tight",
        className,
      )}
      data-slot="card-title"
      {...props}
    >
      {children}
    </h3>
  );
}

function CardDescription({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      className={cn("text-sm text-default-500", className)}
      data-slot="card-description"
      {...props}
    />
  );
}

function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("p-6", className)}
      data-slot="card-content"
      {...props}
    />
  );
}

function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("flex items-center p-6 pt-0", className)}
      data-slot="card-footer"
      {...props}
    />
  );
}

export {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
};
