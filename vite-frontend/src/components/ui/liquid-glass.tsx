import React, { useRef, useState, useEffect, useId } from "react";
import { cn } from "@/lib/utils";
import { displacementMap } from "./displacement-map";

const GlassFilter = ({
  id,
  displacementScale,
  aberrationIntensity,
}: {
  id: string;
  displacementScale: number;
  aberrationIntensity: number;
}) => (
  <svg style={{ position: "absolute", width: "100%", height: "100%", pointerEvents: "none" }} aria-hidden="true">
    <defs>
      <radialGradient id={`${id}-edge-mask`} cx="50%" cy="50%" r="50%">
        <stop offset="0%" stopColor="black" stopOpacity="0" />
        <stop offset={`${Math.max(30, 80 - aberrationIntensity * 2)}%`} stopColor="black" stopOpacity="0" />
        <stop offset="100%" stopColor="white" stopOpacity="1" />
      </radialGradient>
      <filter id={id} x="-35%" y="-35%" width="170%" height="170%" colorInterpolationFilters="sRGB">
        <feImage x="0" y="0" width="100%" height="100%" result="DISPLACEMENT_MAP" href={displacementMap} preserveAspectRatio="xMidYMid slice" />
        <feColorMatrix in="DISPLACEMENT_MAP" type="matrix" values="0.3 0.3 0.3 0 0
0.3 0.3 0.3 0 0
0.3 0.3 0.3 0 0
0 0 0 1 0" result="EDGE_INTENSITY" />
        <feComponentTransfer in="EDGE_INTENSITY" result="EDGE_MASK">
          <feFuncA type="discrete" tableValues={`0 ${aberrationIntensity * 0.05} 1`} />
        </feComponentTransfer>
        <feOffset in="SourceGraphic" dx="0" dy="0" result="CENTER_ORIGINAL" />
        
        {/* Red Channel Displacement */}
        <feDisplacementMap in="SourceGraphic" in2="DISPLACEMENT_MAP" scale={-displacementScale} xChannelSelector="R" yChannelSelector="B" result="RED_DISPLACED" />
        <feColorMatrix in="RED_DISPLACED" type="matrix" values="1 0 0 0 0
0 0 0 0 0
0 0 0 0 0
0 0 0 1 0" result="RED_CHANNEL" />
        
        {/* Green Channel Displacement */}
        <feDisplacementMap in="SourceGraphic" in2="DISPLACEMENT_MAP" scale={-displacementScale - (aberrationIntensity * 0.05)} xChannelSelector="R" yChannelSelector="B" result="GREEN_DISPLACED" />
        <feColorMatrix in="GREEN_DISPLACED" type="matrix" values="0 0 0 0 0
0 1 0 0 0
0 0 0 0 0
0 0 0 1 0" result="GREEN_CHANNEL" />
        
        {/* Blue Channel Displacement */}
        <feDisplacementMap in="SourceGraphic" in2="DISPLACEMENT_MAP" scale={-displacementScale - (aberrationIntensity * 0.1)} xChannelSelector="R" yChannelSelector="B" result="BLUE_DISPLACED" />
        <feColorMatrix in="BLUE_DISPLACED" type="matrix" values="0 0 0 0 0
0 0 0 0 0
0 0 1 0 0
0 0 0 1 0" result="BLUE_CHANNEL" />
        
        <feBlend in="GREEN_CHANNEL" in2="BLUE_CHANNEL" mode="screen" result="GB_COMBINED" />
        <feBlend in="RED_CHANNEL" in2="GB_COMBINED" mode="screen" result="RGB_COMBINED" />
        
        <feGaussianBlur in="RGB_COMBINED" stdDeviation={Math.max(0.1, 0.5 - aberrationIntensity * 0.1)} result="ABERRATED_BLURRED" />
        <feComposite in="ABERRATED_BLURRED" in2="EDGE_MASK" operator="in" result="EDGE_ABERRATION" />
        
        <feComponentTransfer in="EDGE_MASK" result="INVERTED_MASK">
          <feFuncA type="table" tableValues="1 0" />
        </feComponentTransfer>
        <feComposite in="CENTER_ORIGINAL" in2="INVERTED_MASK" operator="in" result="CENTER_CLEAN" />
        <feComposite in="EDGE_ABERRATION" in2="CENTER_CLEAN" operator="over" />
      </filter>
    </defs>
  </svg>
);

export interface LiquidGlassProps extends React.ComponentProps<"div"> {
  displacementScale?: number;
  blurAmount?: number;
  saturation?: number;
  aberrationIntensity?: number;
  cornerRadius?: number | string;
  glassClassName?: string;
  wrapperClassName?: string;
}

export function LiquidGlass({
  children,
  className,
  glassClassName,
  wrapperClassName,
  style,
  displacementScale = 25,
  blurAmount = 0.5,
  saturation = 180,
  aberrationIntensity = 2,
  cornerRadius = 24,
  ...props
}: LiquidGlassProps) {
  const filterId = useId().replace(/:/g, "-");
  const glassRef = useRef<HTMLDivElement>(null);
  
  const [isFirefox, setIsFirefox] = useState(false);

  useEffect(() => {
    setIsFirefox(navigator.userAgent.toLowerCase().includes("firefox"));
  }, []);

  return (
    <div
      className={cn("relative flex flex-col", wrapperClassName)}
      style={style}
      {...props}
    >
      <GlassFilter 
        id={filterId} 
        displacementScale={displacementScale} 
        aberrationIntensity={aberrationIntensity} 
      />
      
      <div
        ref={glassRef}
        className={cn(
          "absolute inset-0 z-0 overflow-hidden pointer-events-none",
          glassClassName
        )}
        style={{
          borderRadius: cornerRadius,
          /* A much better, more subtle glass highlight bounding box */
          boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.2), inset 0 1px 1px rgba(255,255,255,0.5), inset 0 -1px 1px rgba(0,0,0,0.1)",
          background: "linear-gradient(135deg, rgba(255,255,255,0.2) 0%, rgba(255,255,255,0.05) 100%)",
        }}
      >
        <div
          className="absolute inset-0 z-0"
          style={{
            filter: isFirefox ? "none" : `url(#${filterId})`,
            backdropFilter: `blur(${blurAmount * 32}px) saturate(${saturation}%)`,
            WebkitBackdropFilter: `blur(${blurAmount * 32}px) saturate(${saturation}%)`,
          }}
        />
      </div>

      <div className={cn("relative z-10 flex flex-col flex-1", className)}>
        {children}
      </div>
    </div>
  );
}