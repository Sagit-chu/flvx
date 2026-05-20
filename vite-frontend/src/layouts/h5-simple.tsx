import React from "react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/shadcn-bridge/heroui/button";
import { BackIcon } from "@/components/icons";
import { BrandLogo } from "@/components/brand-logo";
import { siteConfig } from "@/config/site";
import { useScrollTopOnPathChange } from "@/hooks/useScrollTopOnPathChange";

export default function H5SimpleLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const navigate = useNavigate();

  useScrollTopOnPathChange();

  const handleBack = () => {
    navigate("/profile");
  };

  return (
    <div className="flex flex-col min-h-screen bg-mesh-gradient">
      {/* 顶部导航栏 */}
      <header className="safe-top relative z-10 flex h-14 flex-shrink-0 items-center justify-between border-b border-divider bg-surface/95 px-4 shadow-sm">
        <div className="flex items-center gap-2">
          <Button isIconOnly size="sm" variant="light" onPress={handleBack}>
            <BackIcon className="w-5 h-5" />
          </Button>
          <BrandLogo size={20} />
          <h1 className="text-sm font-bold text-foreground">
            {siteConfig.name}
          </h1>
        </div>

        <div className="flex items-center gap-2" />
      </header>

      {/* 主内容区域 */}
      <main className="flex-1 pb-0">{children}</main>
    </div>
  );
}
