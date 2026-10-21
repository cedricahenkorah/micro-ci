import { Suspense } from "react";
import { notFound } from "next/navigation";
import { RunDetail } from "@/components/run-detail";
import { LoadingRows } from "@/components/query-state";

export const metadata = { title: "Run details" };

type RunPageProps = {
  params: Promise<{ id: string }>;
};

export default function RunPage({ params }: RunPageProps) {
  return (
    <Suspense fallback={<LoadingRows />}>
      <RunPageContent params={params} />
    </Suspense>
  );
}

async function RunPageContent({ params }: RunPageProps) {
  const { id } = await params;
  const runId = Number(id);

  if (!/^\d+$/.test(id) || !Number.isSafeInteger(runId) || runId <= 0) {
    notFound();
  }

  return (
    <RunDetail
      key={runId}
      id={runId}
    />
  );
}
