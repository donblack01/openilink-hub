import { cn } from "../../lib/utils";
import { type InputHTMLAttributes } from "react";

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1",
        "text-sm shadow-xs transition-colors",
        "file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground",
        "placeholder:text-muted-foreground",
        "focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:border-ring",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  );
}
