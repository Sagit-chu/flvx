import * as React from "react";
import { cn } from "@/lib/utils";

// Generate a static unique ID for the SVG filter
const FILTER_ID = "flvx-liquid-glass-filter";

function LiquidGlassEffect({ cornerRadius = 24 }: { cornerRadius?: number }) {
  return (
    <>
      <svg
        style={{ width: 0, height: 0, position: "absolute" }}
        aria-hidden="true"
      >
        <defs>
          <filter
            id={FILTER_ID}
            x="-20%"
            y="-20%"
            width="140%"
            height="140%"
            colorInterpolationFilters="sRGB"
          >
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.015"
              numOctaves="3"
              result="NOISE"
            />
            <feDisplacementMap
              in="SourceGraphic"
              in2="NOISE"
              scale="15"
              xChannelSelector="R"
              yChannelSelector="G"
              result="DISPLACED"
            />
            <feGaussianBlur in="DISPLACED" stdDeviation="0.5" result="BLURRED" />
            <feComposite in="BLURRED" in2="SourceGraphic" operator="in" />
          </filter>
        </defs>
      </svg>
      {/* Inner Glossy Highlights */}
      <div
        className="pointer-events-none absolute inset-0 mix-blend-overlay opacity-50 z-20"
        style={{
          borderRadius: cornerRadius,
          boxShadow:
            "inset 0 1px 1px rgba(255,255,255,0.8), inset 0 0 0 1px rgba(255,255,255,0.4), inset 0 -1px 1px rgba(0,0,0,0.1)",
          background:
            "linear-gradient(135deg, rgba(255,255,255,0.4) 0%, rgba(255,255,255,0) 40%, rgba(255,255,255,0) 60%, rgba(255,255,255,0.1) 100%)",
        }}
      />
    </>
  );
}

function Card({ className, style, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "relative flex flex-col group transition-all duration-300",
        "rounded-[24px] border border-white/80 dark:border-white/10",
        "bg-white/40 dark:bg-zinc-900/40 backdrop-blur-2xl shadow-[0_15px_35px_rgba(0,0,0,0.1)]",
        "text-card-foreground",
        className,
      )}
      style={{
        ...style,
        filter: `url(#${FILTER_ID})`,
        WebkitBackdropFilter: "blur(24px) saturate(180%)",
        backdropFilter: "blur(24px) saturate(180%)",
      }}
      data-slot="card"
    >
      <LiquidGlassEffect cornerRadius={24} />
      <div className="relative z-10 flex flex-col flex-1 w-full h-full" {...props} />
    </div>
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
