import { useEffect, useState } from "react";

import { isWebViewFunc } from "@/utils/panel";

export const useWebViewMode = (): boolean => {
  const [isWebView, setIsWebView] = useState(false);

  useEffect(() => {
    setIsWebView(isWebViewFunc());
  }, []);

  return isWebView;
};
