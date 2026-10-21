import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-colors disabled:pointer-events-none disabled:opacity-55 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        primary: "bg-steel text-white hover:bg-steel-deep shadow-[0_8px_20px_-12px_rgb(43_103_168/0.9)]",
        secondary: "glass-strong text-ink hover:bg-white",
        ghost: "text-ink-soft hover:bg-frost hover:text-ink",
        outline: "border border-line bg-white/60 text-ink hover:border-glacier hover:bg-white",
        danger: "text-risk hover:bg-risk-bg",
        link: "text-steel underline-offset-4 hover:underline px-0 h-auto",
      },
      size: {
        sm: "h-8 rounded-[10px] px-3 text-[13px]",
        md: "h-10 rounded-[12px] px-4 text-sm",
        lg: "h-12 rounded-[14px] px-6 text-[15px]",
        icon: "size-9 rounded-[10px]",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  loading?: boolean;
}

export function Button({ className, variant, size, asChild, loading, children, disabled, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp className={cn(buttonVariants({ variant, size }), className)} disabled={disabled || loading} {...props}>
      {asChild ? (
        children
      ) : (
        <>
          {loading && <Loader2 className="animate-spin" aria-hidden />}
          {children}
        </>
      )}
    </Comp>
  );
}

export { buttonVariants };
