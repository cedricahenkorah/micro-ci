import { Badge } from "@/components/ui/badge";
import type { RunStatus, StepStatus } from "@/lib/api/types";

const labels: Record<RunStatus | StepStatus, string> = {
  queued: "Queued",
  running: "Running",
  succeeded: "Succeeded",
  failed: "Failed",
  timed_out: "Timed out",
  cancelled: "Cancelled",
  pending: "Pending",
  skipped: "Skipped",
  lost: "Lost",
};

export function StatusBadge({ status }: { status: RunStatus | StepStatus }) {
  return (
    <Badge
      variant="outline"
      className={`status-badge status-${status}`}
    >
      <span className="status-dot" />
      {labels[status]}
    </Badge>
  );
}
