import { Toaster } from "sonner";
import { GitBranch } from "lucide-react";
import { ThemeToggle, LangToggle } from "@/components/header-controls";

export function ShellHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center gap-2">
          <span className="flex items-center gap-2 text-lg font-bold">
            <GitBranch className="h-5 w-5" />
            gitdash Admin
          </span>
          <div className="ml-auto flex items-center gap-1">
            <ThemeToggle />
            <LangToggle />
          </div>
        </div>
      </header>
      <main className="container py-6">{children}</main>
      <Toaster richColors position="top-center" />
    </div>
  );
}

