import type { DragEndEvent } from "@dnd-kit/core";
import type { OrderUpdatePayload } from "@/api";

import { useCallback, useEffect, useRef, useState } from "react";
import { arrayMove } from "@dnd-kit/sortable";

interface Result {
  code: number;
  msg?: string;
}

// Keep one complete, authoritative snapshot. Filtering is presentation-only.
export function useServerOrder<T extends { id: number }>(
  fetchItems: () => Promise<T[]>,
  saveOrder: (payload: OrderUpdatePayload) => Promise<Result>,
) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(false);
  const request = useRef(0);
  const savingRef = useRef(false);
  const snapshot = useRef(items);

  snapshot.current = items;
  const dragSnapshot = useRef<T[] | null>(null);

  const reload = useCallback(async () => {
    const version = ++request.current;

    setLoading(true);
    try {
      const rows = await fetchItems();

      if (!mounted.current || version !== request.current) return;
      setItems(rows);
    } catch {
      if (!mounted.current || version !== request.current) return;
      // Never expose a partial or unverified set as reorderable after failure.
      setItems([]);
      setError("加载列表失败，请重试后再排序。");
    } finally {
      if (mounted.current && version === request.current) setLoading(false);
    }
  }, [fetchItems]);

  useEffect(() => {
    mounted.current = true;
    void reload();

    return () => {
      mounted.current = false;
      ++request.current;
    };
  }, [reload]);

  const onDragStart = () => {
    dragSnapshot.current = snapshot.current;
  };
  const reorder = async ({ active, over }: DragEndEvent, disabled: boolean) => {
    const before = dragSnapshot.current;

    dragSnapshot.current = null;
    if (
      disabled ||
      loading ||
      savingRef.current ||
      !over ||
      active.id === over.id ||
      before !== snapshot.current
    )
      return;
    const from = items.findIndex((item) => item.id === active.id);
    const to = items.findIndex((item) => item.id === over.id);

    if (from < 0 || to < 0) return;
    savingRef.current = true;
    setSaving(true);
    setError("");
    try {
      const result = await saveOrder({
        ids: arrayMove(items, from, to).map((item) => item.id),
        expectedIds: items.map((item) => item.id),
      });

      if (!mounted.current) return;
      if (result.code !== 0) {
        setError(
          result.code === 409
            ? "列表已被其他操作更改，排序未保存，已重新获取服务器顺序。"
            : `排序保存失败：${result.msg || "请重试"}。已重新获取服务器顺序。`,
        );
      }
    } catch {
      if (mounted.current)
        setError("排序保存未确认，已重新获取服务器顺序，请检查后重试。");
    } finally {
      if (mounted.current) {
        await reload();
        if (mounted.current) setSaving(false);
      }
      savingRef.current = false;
    }
  };

  return {
    items,
    setItems,
    loading,
    saving,
    error,
    reload,
    onDragStart,
    reorder,
  };
}
