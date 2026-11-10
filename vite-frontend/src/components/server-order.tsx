import { createContext, useContext, type ReactNode } from "react";
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";

import { TableRow } from "@/shadcn-bridge/heroui/table";

const HandleContext = createContext<ReactNode>(null);

export function OrderHandle() {
  return <>{useContext(HandleContext)}</>;
}

export function OrderItem({
  id,
  label,
  disabled,
  row = false,
  children,
}: {
  id: number;
  label: string;
  disabled: boolean;
  row?: boolean;
  children: ReactNode;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled });
  const Component = row ? TableRow : "div";

  return (
    <HandleContext.Provider
      value={
        <button
          ref={setActivatorNodeRef}
          {...attributes}
          {...listeners}
          aria-label={`拖动排序：${label}`}
          className={`inline-flex items-center justify-center shrink-0 rounded text-default-500 hover:bg-default-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary disabled:opacity-40 disabled:cursor-not-allowed cursor-grab active:cursor-grabbing ${row ? "p-1" : "h-8 w-8 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"}`}
          disabled={disabled}
          style={{ touchAction: "none" }}
          type="button"
        >
          <GripVertical aria-hidden="true" className="h-4 w-4" />
        </button>
      }
    >
      <Component
        ref={setNodeRef}
        className={row ? undefined : "h-full min-w-0"}
        style={{
          transform: CSS.Transform.toString(transform),
          transition,
          opacity: isDragging ? 0.5 : 1,
          position: "relative",
          zIndex: isDragging ? 1 : undefined,
        }}
      >
        {children}
      </Component>
    </HandleContext.Provider>
  );
}

export function OrderList({
  ids,
  children,
  onDragStart,
  onDragEnd,
}: {
  ids: number[];
  children: ReactNode;
  onDragStart: () => void;
  onDragEnd: (event: DragEndEvent) => void;
}) {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  return (
    <DndContext
      accessibility={{
        screenReaderInstructions: {
          draggable: "按空格开始排序，方向键移动，再按空格保存，Escape 取消。",
        },
      }}
      collisionDetection={closestCenter}
      sensors={sensors}
      onDragEnd={onDragEnd}
      onDragStart={onDragStart}
    >
      <SortableContext items={ids} strategy={rectSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  );
}

export function OrderStatus({
  reason,
  error,
  retry,
}: {
  reason: string;
  error: string;
  retry: () => void;
}) {
  return (
    <div className="mb-3 text-sm text-default-500">
      <p role="status">
        {reason || "拖动手柄调整全局顺序；键盘可按空格、方向键、空格完成排序。"}
      </p>
      {error && (
        <p className="text-danger mt-2" role="alert">
          {error}{" "}
          <button className="underline" type="button" onClick={retry}>
            重新加载
          </button>
        </p>
      )}
    </div>
  );
}
