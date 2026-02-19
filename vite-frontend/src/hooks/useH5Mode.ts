import { useEffect, useState } from "react";

import { DEFAULT_MOBILE_BREAKPOINT } from "@/hooks/useMobileBreakpoint";

const detectH5Mode = (): boolean => {
  const isMobile = window.innerWidth <= DEFAULT_MOBILE_BREAKPOINT;
  const isMobileBrowser =
    /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
      navigator.userAgent,
    );
  const urlParams = new URLSearchParams(window.location.search);
  const isH5Param = urlParams.get("h5") === "true";

  return isMobile || isMobileBrowser || isH5Param;
};

export const useH5Mode = (): boolean => {
  const [isH5, setIsH5] = useState(detectH5Mode);

  useEffect(() => {
    const checkH5Mode = () => {
      setIsH5(detectH5Mode());
    };

    window.addEventListener("resize", checkH5Mode);

    return () => window.removeEventListener("resize", checkH5Mode);
  }, []);

  return isH5;
};
