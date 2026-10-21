import Link from "next/link";
import { GitBranch, Layers2, Terminal } from "lucide-react";
import { RunGuide } from "./run-guide";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="app-shell">
      <a
        href="#main"
        className="skip-link"
      >
        Skip to content
      </a>

      <aside className="sidebar">
        <Link
          href="/runs"
          className="brand"
        >
          <span className="brand-icon">
            <Terminal
              size={18}
              strokeWidth={2}
            />
          </span>

          <span className="brand-wordmark">
            micro<span className="brand-muted">ci</span>
          </span>
        </Link>

        <nav aria-label="Main navigation">
          <Link
            href="/runs"
            className="nav-item"
            aria-current="page"
          >
            <Layers2 size={16} />
            Pipeline runs
          </Link>
        </nav>

        <div className="sidebar-bottom">
          <div className="local-label">
            <GitBranch size={14} />
            Built for your commits.
          </div>

          <RunGuide compact />
        </div>
      </aside>

      <main
        id="main"
        className="main-content"
      >
        {children}
      </main>
    </div>
  );
}
