import { useEffect, useState } from "react";

export const DEFAULT_MOBILE_BREAKPOINT = 768;

export const useMobileBreakpoint = (
  breakpoint = DEFAULT_MOBILE_BREAKPOINT,
): boolean => {
  const [isMobile, setIsMobile] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }

    return window.innerWidth < breakpoint;
  });

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const onResize = () => {
      setIsMobile(window.innerWidth < breakpoint);
    };

    window.addEventListener("resize", onResize);

    return () => {
      window.removeEventListener("resize", onResize);
    };
  }, [breakpoint]);

  return isMobile;
};
