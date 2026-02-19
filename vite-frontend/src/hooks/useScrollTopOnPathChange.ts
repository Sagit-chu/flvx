import { useEffect } from "react";
import { useLocation } from "react-router-dom";

export const useScrollTopOnPathChange = (): void => {
  const { pathname } = useLocation();

  useEffect(() => {
    if (!pathname) {
      return;
    }

    try {
      window.scrollTo({ top: 0, left: 0, behavior: "auto" });
    } catch {
      window.scrollTo(0, 0);
    }

    document.body.scrollTop = 0;
    document.documentElement.scrollTop = 0;
  }, [pathname]);
};
