import { cn } from "../../lib/utils";
import type { HTMLAttributes } from "react";

export function Card({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("rounded-xl border bg-card text-card-foreground shadow-sm p-6", className)}
      {...props}
    />
  );
}
