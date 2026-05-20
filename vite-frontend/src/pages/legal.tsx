import { useEffect, useMemo, useState } from "react";

import { getLegalPages, type LegalPagesApiData } from "@/api";

const titles: Record<keyof LegalPagesApiData, string> = {
  terms: "服务条款",
  privacy: "隐私政策",
  refundPolicy: "退款政策",
  acceptableUse: "可接受使用政策",
};

export default function LegalPage({ type }: { type: keyof LegalPagesApiData }) {
  const [data, setData] = useState<LegalPagesApiData | null>(null);

  useEffect(() => {
    void getLegalPages().then((res) => {
      if (res.code === 0) setData(res.data || null);
    });
  }, []);

  const content = useMemo(() => data?.[type] || "", [data, type]);

  return (
    <main className="min-h-screen bg-background px-4 py-10 text-foreground">
      <section className="native-panel mx-auto max-w-3xl p-6">
        <h1 className="text-2xl font-bold">{titles[type]}</h1>
        <div className="mt-5 whitespace-pre-wrap text-sm leading-7 text-default-700">
          {content || "暂未配置内容"}
        </div>
      </section>
    </main>
  );
}
