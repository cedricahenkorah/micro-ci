"use client";

import { Terminal, ArrowUpRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function RunGuide({ compact = false }: { compact?: boolean }) {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          variant={compact ? "ghost" : "outline"}
          size="sm"
        >
          <Terminal />
          {compact ? "Quick start" : "Run a pipeline"}
          {!compact && <ArrowUpRight />}
        </Button>
      </DialogTrigger>

      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>From commit to result.</DialogTitle>

          <DialogDescription>
            Start a run from a local repository using the Micro CI CLI.
          </DialogDescription>
        </DialogHeader>

        <ol className="guide-steps">
          <li>
            <span>01</span>

            <div>
              <strong>Define your pipeline</strong>

              <p>
                Add a <code>.micro-ci.yml</code> file to your repository with an
                image and sequential steps.
              </p>
            </div>
          </li>

          <li>
            <span>02</span>

            <div>
              <strong>Commit your changes</strong>

              <p>
                Runs use the current committed revision. Uncommitted changes
                stay on your machine.
              </p>
            </div>
          </li>

          <li>
            <span>03</span>

            <div>
              <strong>Start your run</strong>

              <p>
                With the API and worker running, execute this in your
                repository:
              </p>

              <pre className="command-block">micro-ci run .</pre>
            </div>
          </li>
        </ol>

        <p className="text-sm text-muted-foreground">
          Connected GitHub pushes also appear here automatically.
        </p>
      </DialogContent>
    </Dialog>
  );
}
