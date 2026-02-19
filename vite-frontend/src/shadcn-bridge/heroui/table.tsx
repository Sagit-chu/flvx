import * as React from "react";

import { cn } from "@/lib/utils";

interface TableClassNames {
  th?: string;
  wrapper?: string;
}

interface TableStyleContextValue {
  thClassName?: string;
}

const TableStyleContext = React.createContext<TableStyleContextValue>({});

export interface TableProps extends React.ComponentProps<"table"> {
  classNames?: TableClassNames;
}

export function Table({ children, className, classNames, ...props }: TableProps) {
  return (
    <TableStyleContext.Provider value={{ thClassName: classNames?.th }}>
      <div className={cn("w-full overflow-auto rounded-md", classNames?.wrapper)}>
        <table className={cn("w-full text-sm", className)} {...props}>
          {children}
        </table>
      </div>
    </TableStyleContext.Provider>
  );
}

export function TableHeader({ children, className, ...props }: React.ComponentProps<"thead">) {
  const childArray = React.Children.toArray(children);
  const hasRow = childArray.some(
    (child) => React.isValidElement(child) && child.type === TableRow,
  );

  return (
    <thead className={cn("border-b bg-default-50/60", className)} {...props}>
      {hasRow ? children : <TableRow>{children}</TableRow>}
    </thead>
  );
}

interface TableBodyProps<T> extends Omit<React.ComponentProps<"tbody">, "children"> {
  children?: React.ReactNode | ((item: T) => React.ReactNode);
  emptyContent?: React.ReactNode;
  isLoading?: boolean;
  items?: T[];
  loadingContent?: React.ReactNode;
}

export function TableBody<T>({
  children,
  className,
  emptyContent,
  isLoading,
  items,
  loadingContent,
  ...props
}: TableBodyProps<T>) {
  if (isLoading) {
    return (
      <tbody className={className} {...props}>
        <tr>
          <td className="p-4 text-center text-default-500" colSpan={999}>
            {loadingContent ?? "加载中..."}
          </td>
        </tr>
      </tbody>
    );
  }

  if (items && typeof children === "function") {
    if (items.length === 0) {
      return (
        <tbody className={className} {...props}>
          <tr>
            <td className="p-4 text-center text-default-500" colSpan={999}>
              {emptyContent ?? "暂无数据"}
            </td>
          </tr>
        </tbody>
      );
    }

    return (
      <tbody className={className} {...props}>
        {items.map((item) => {
          const key =
            typeof item === "object" && item !== null && "id" in item
              ? String((item as { id: React.Key }).id)
              : JSON.stringify(item);

          return <React.Fragment key={key}>{children(item)}</React.Fragment>;
        })}
      </tbody>
    );
  }

  const staticChildren =
    typeof children === "function" ? [] : React.Children.toArray(children);

  if (staticChildren.length === 0) {
    return (
      <tbody className={className} {...props}>
        <tr>
          <td className="p-4 text-center text-default-500" colSpan={999}>
            {emptyContent ?? "暂无数据"}
          </td>
        </tr>
      </tbody>
    );
  }

  return (
    <tbody className={className} {...props}>
      {typeof children === "function" ? null : children}
    </tbody>
  );
}

export function TableColumn({ className, ...props }: React.ComponentProps<"th">) {
  const { thClassName } = React.useContext(TableStyleContext);

  return (
    <th
      className={cn("px-3 py-2 text-left font-medium text-default-600", thClassName, className)}
      {...props}
    />
  );
}

export function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return <tr className={cn("border-b last:border-b-0", className)} {...props} />;
}

export function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return <td className={cn("px-3 py-2 align-middle", className)} {...props} />;
}
