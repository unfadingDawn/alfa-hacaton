"use client";

import * as React from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;

export function DialogContent({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-ink/25 backdrop-blur-[2px] data-[state=open]:animate-in" />
      <DialogPrimitive.Content
        className={cn(
          "glass-strong fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[calc(100vw-32px)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-lg)] p-6",
          className,
        )}
      >
        <DialogPrimitive.Title className="pr-8 text-lg font-semibold tracking-[-0.01em]">{title}</DialogPrimitive.Title>
        {description && <DialogPrimitive.Description className="mt-1 text-sm text-ink-soft">{description}</DialogPrimitive.Description>}
        <div className="mt-5">{children}</div>
        <DialogPrimitive.Close className="absolute right-4 top-4 rounded-md p-1 text-silver hover:bg-frost hover:text-ink" aria-label="Закрыть">
          <X className="size-4" />
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}
