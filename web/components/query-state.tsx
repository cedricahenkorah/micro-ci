import { AlertCircle, Layers2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

export function QueryError({
  error,
  retry,
}: {
  error: Error;
  retry: () => void;
}) {
  return (
    <div
      role="alert"
      className="error-panel"
    >
      <AlertCircle size={18} />

      <div>
        <strong>Something went wrong</strong>
        <p>{error.message}</p>
      </div>

      <Button
        variant="outline"
        size="sm"
        onClick={retry}
      >
        Try again
      </Button>
    </div>
  );
}

export function LoadingRows() {
  return (
    <div
      aria-label="Loading runs"
      role="status"
      className="loading-rows"
    >
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i}>
          <Skeleton className="h-9 w-9 rounded-lg" />
          <Skeleton className="h-4 w-1/3" />
          <Skeleton className="ml-auto h-5 w-20" />
        </div>
      ))}
    </div>
  );
}

export function EmptyState({ filtered = false }: { filtered?: boolean }) {
  return (
    <div className="empty-state">
      <span className="empty-icon">
        <Layers2
          size={25}
          strokeWidth={1.5}
        />
      </span>

      <h2>{filtered ? "No matching runs" : "Your next commit starts here"}</h2>

      <p>
        {filtered
          ? "Try a different search or status on this page."
          : "Run a committed repository with the CLI, or push to your connected GitHub repository."}
      </p>

      {!filtered && <code>micro-ci run .</code>}
    </div>
  );
}
