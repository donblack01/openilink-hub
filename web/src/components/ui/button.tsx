import { cn } from "../../lib/utils";
import { type ButtonHTMLAttributes } from "react";

type Variant = "default" | "secondary" | "destructive" | "ghost" | "outline" | "link";

const variants: Record<Variant, string> = {
  default: "bg-primary text-primary-foreground shadow-xs hover:bg-primary/90",
  secondary: "bg-secondary text-secondary-foreground shadow-xs hover:bg-secondary/80",
  destructive: "bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90",
  ghost: "hover:bg-accent hover:text-accent-foreground",
  outline: "border border-input bg-background shadow-xs hover:bg-accent hover:text-accent-foreground",
  link: "text-primary underline-offset-4 hover:underline",
};

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: "sm" | "md" | "lg" | "icon";
}

export function Button({ className, variant = "default", size = "md", ...props }: ButtonProps) {
  const sizeClass = {
    sm: "h-8 rounded-md gap-1.5 px-3 text-xs",
    md: "h-9 px-4 py-2 rounded-md text-sm",
    lg: "h-10 rounded-md px-6 text-base",
    icon: "size-9 rounded-md",
  }[size];
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium",
        "transition-all cursor-pointer",
        "disabled:pointer-events-none disabled:opacity-50",
        "focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
        variants[variant], sizeClass, className
      )}
      {...props}
    />
  );
}
