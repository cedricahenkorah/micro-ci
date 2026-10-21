"use client";

import Link from "next/link";
import { useState } from "react";
import {
  ArrowDown,
  ChevronLeft,
  ChevronRight,
  GitBranch,
  GitCommitHorizontal,
  RefreshCw,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useRuns } from "@/hooks/use-runs";
import { useNow } from "@/hooks/use-now";
import { dateTime, duration, relativeTime, repositoryName } from "@/lib/format";
import { StatusBadge } from "./status-badge";
import { RunGuide } from "./run-guide";
import { EmptyState, LoadingRows, QueryError } from "./query-state";

const statusOptions = [
  { value: "queued", label: "Queued" },
  { value: "running", label: "Running" },
  { value: "succeeded", label: "Succeeded" },
  { value: "failed", label: "Failed" },
  { value: "timed_out", label: "Timed out" },
  { value: "cancelled", label: "Cancelled" },
];

export function RunsList() {
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const query = useRuns(page);
  const now = useNow();
  const data = query.data;
  const searchTerm = search.toLowerCase();

  const runs =
    data?.data.filter((run) => {
      const matchesStatus = status === "all" || run.status === status;

      const searchableText =
        `${run.id} ${run.repository} ${run.branch} ${run.commit_sha}`.toLowerCase();

      return matchesStatus && searchableText.includes(searchTerm);
    }) ?? [];

  const filtered = search !== "" || status !== "all";
  const firstRun = data?.totalCount ? page * data.itemsPerPage + 1 : 0;

  const lastRun = data
    ? Math.min((page + 1) * data.itemsPerPage, data.totalCount)
    : 0;

  const resultSummary = filtered
    ? `${runs.length} matching on this page`
    : `${firstRun}–${lastRun} of ${data?.totalCount ?? 0} runs`;

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Pipeline runs</h1>
        </div>

        <RunGuide />
      </header>

      <div className="section-bar">
        <span className="section-tab">
          All runs <span className="count">{data?.totalCount ?? "—"}</span>
        </span>
      </div>

      <div className="toolbar">
        <div className="search-field">
          <Search size={15} />

          <Input
            aria-label="Search runs on this page"
            placeholder="Search this page by repository, branch, or commit…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>

        <Select
          value={status}
          onValueChange={setStatus}
        >
          <SelectTrigger
            className="status-select"
            aria-label="Filter this page by status"
          >
            <SelectValue />
          </SelectTrigger>

          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>

            {statusOptions.map((option) => (
              <SelectItem
                key={option.value}
                value={option.value}
              >
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          size="icon"
          aria-label="Refresh runs"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw
            size={15}
            className={query.isFetching ? "animate-spin" : ""}
          />
        </Button>
      </div>

      {query.isError && (
        <QueryError
          error={query.error}
          retry={() => query.refetch()}
        />
      )}

      {query.isPending ? (
        <LoadingRows />
      ) : (
        data && (
          <>
            <div className="runs-table">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Run</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Branch / Commit</TableHead>
                    <TableHead>Duration</TableHead>

                    <TableHead className="text-right">
                      <span className="inline-flex items-center gap-1">
                        Created <ArrowDown size={12} />
                      </span>
                    </TableHead>

                    <TableHead className="w-8">
                      <span className="sr-only">Open</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>

                <TableBody>
                  {runs.map((run) => (
                    <TableRow key={run.id}>
                      <TableCell>
                        <Link
                          href={`/runs/${run.id}`}
                          className="run-link"
                        >
                          <span className={`run-icon status-${run.status}`}>
                            <GitCommitHorizontal size={19} />
                          </span>

                          <span>
                            <strong title={run.repository}>
                              {repositoryName(run.repository)}
                            </strong>

                            <span className="row-secondary">
                              Run #{run.id}
                              {run.retry_of_run_id
                                ? ` · Retry of #${run.retry_of_run_id}`
                                : ""}
                            </span>
                          </span>
                        </Link>
                      </TableCell>

                      <TableCell>
                        <StatusBadge status={run.status} />

                        {run.cancel_requested_at &&
                          run.status === "running" && (
                            <span className="row-secondary">Cancelling…</span>
                          )}
                      </TableCell>

                      <TableCell>
                        <span className="branch">
                          <GitBranch size={13} />
                          {run.branch}
                        </span>

                        <code className="row-secondary">
                          {run.commit_sha.slice(0, 7)}
                        </code>
                      </TableCell>

                      <TableCell className="tabular-nums text-muted-foreground">
                        {duration(run.started_at, run.finished_at, now)}
                      </TableCell>

                      <TableCell className="text-right text-muted-foreground">
                        <time
                          dateTime={run.created_at}
                          title={dateTime(run.created_at)}
                        >
                          {relativeTime(run.created_at, now)}
                        </time>
                      </TableCell>

                      <TableCell>
                        <Link
                          href={`/runs/${run.id}`}
                          className="row-arrow"
                          aria-label={`View run ${run.id}`}
                        >
                          <ChevronRight size={16} />
                        </Link>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            {runs.length === 0 && <EmptyState filtered={filtered} />}

            <footer className="table-footer">
              <span>{resultSummary}</span>

              <div>
                <span>
                  Page {page + 1} of {Math.max(1, data.totalPages)}
                </span>

                <Button
                  variant="outline"
                  size="sm"
                  disabled={page === 0 || query.isPlaceholderData}
                  onClick={() => setPage(page - 1)}
                >
                  <ChevronLeft />
                  Newer
                </Button>

                <Button
                  variant="outline"
                  size="sm"
                  disabled={!data.hasMore || query.isPlaceholderData}
                  onClick={() => setPage(page + 1)}
                >
                  Older
                  <ChevronRight />
                </Button>
              </div>
            </footer>
          </>
        )
      )}
    </>
  );
}
