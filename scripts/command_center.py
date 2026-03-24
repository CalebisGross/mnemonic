#!/usr/bin/env python3
"""
Mnemonic Command Center -- GitHub project dashboard.

Serves a live web dashboard tracking open PRs and issues for
appsprout-dev/mnemonic. Uses `gh` CLI for all GitHub data.

Usage:
    python3 command_center.py              # start on default port 8042
    python3 command_center.py --port 9090  # custom port
    python3 command_center.py --no-browser # don't auto-open browser

Requires:
    - gh CLI installed and authenticated
    - No Python dependencies beyond stdlib
"""

import argparse
import json
import subprocess
import sys
import threading
import time
import webbrowser
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
from http.server import HTTPServer, SimpleHTTPRequestHandler
from pathlib import Path

REPO = "appsprout-dev/mnemonic"
CACHE_TTL = 30  # seconds
MAX_WORKERS = 4
BOT_LOGINS = frozenset({
    "github-actions[bot]",
    "dependabot[bot]",
    "release-please[bot]",
    "codecov[bot]",
})

# ANSI colors for terminal output
C_RESET = "\033[0m"
C_BOLD = "\033[1m"
C_GREEN = "\033[32m"
C_YELLOW = "\033[33m"
C_CYAN = "\033[36m"
C_RED = "\033[31m"
C_DIM = "\033[2m"

# ---------------------------------------------------------------------------
# Cache
# ---------------------------------------------------------------------------

_cache_lock = threading.Lock()
_cache: dict = {}
_cache_ts: float = 0.0


def _cache_valid() -> bool:
    return (time.time() - _cache_ts) < CACHE_TTL


# ---------------------------------------------------------------------------
# gh helpers
# ---------------------------------------------------------------------------

def gh(args: list[str], timeout: int = 30) -> str:
    """Run a gh CLI command and return stdout."""
    cmd = ["gh"] + args
    result = subprocess.run(
        cmd, capture_output=True, text=True, timeout=timeout
    )
    if result.returncode != 0:
        raise RuntimeError(f"gh failed: {' '.join(cmd)}\n{result.stderr.strip()}")
    return result.stdout.strip()


def gh_json(args: list[str], timeout: int = 30):
    """Run gh and parse JSON output."""
    raw = gh(args, timeout=timeout)
    if not raw:
        return []
    return json.loads(raw)


# ---------------------------------------------------------------------------
# Data fetching
# ---------------------------------------------------------------------------

def fetch_pr_list():
    """Fetch open PRs with basic fields."""
    fields = "number,title,author,state,labels,createdAt,updatedAt,headRefName,baseRefName,isDraft,mergeable"
    return gh_json([
        "pr", "list", "--repo", REPO, "--state", "open",
        "--json", fields, "--limit", "100",
    ])


def fetch_pr_detail(number: int):
    """Fetch detailed status for a single PR: reviews, checks, comments."""
    fields = (
        "number,title,author,state,labels,createdAt,updatedAt,"
        "headRefName,baseRefName,isDraft,mergeable,"
        "reviews,statusCheckRollup,comments,mergeStateStatus"
    )
    return gh_json([
        "pr", "view", str(number), "--repo", REPO, "--json", fields,
    ])


def fetch_issues():
    """Fetch open issues."""
    fields = "number,title,author,labels,createdAt,updatedAt,milestone,assignees"
    return gh_json([
        "issue", "list", "--repo", REPO, "--state", "open",
        "--json", fields, "--limit", "200",
    ])


def fetch_milestones():
    """Fetch milestones via gh api."""
    try:
        raw = gh(["api", f"repos/{REPO}/milestones", "--paginate"])
        if not raw:
            return []
        return json.loads(raw)
    except Exception:
        return []


def fetch_recent_closed(days: int = 30) -> dict:
    """Count recently merged PRs and closed issues within the given window."""
    cutoff_ts = time.time() - (days * 86400)
    merged = 0
    closed_issues = 0

    cutoff_date = datetime.fromtimestamp(cutoff_ts, tz=timezone.utc).strftime("%Y-%m-%d")

    try:
        # Use gh search for date-filtered results
        prs = gh_json([
            "search", "prs", "--repo", REPO,
            "--merged-at", f">{cutoff_date}",
            "--json", "number", "--limit", "500",
        ])
        merged = len(prs)
    except Exception:
        # Fallback: fetch recent merged and count
        try:
            prs = gh_json([
                "pr", "list", "--repo", REPO, "--state", "merged",
                "--json", "number,mergedAt", "--limit", "200",
            ])
            for pr in prs:
                merged_at = pr.get("mergedAt", "")
                if merged_at:
                    ts = datetime.fromisoformat(merged_at.replace("Z", "+00:00")).timestamp()
                    if ts >= cutoff_ts:
                        merged += 1
        except Exception:
            pass

    try:
        issues = gh_json([
            "search", "issues", "--repo", REPO,
            "--closed-at", f">{cutoff_date}",
            "--json", "number", "--limit", "500",
        ])
        closed_issues = len(issues)
    except Exception:
        try:
            issues = gh_json([
                "issue", "list", "--repo", REPO, "--state", "closed",
                "--json", "number,closedAt", "--limit", "200",
            ])
            for iss in issues:
                closed_at = iss.get("closedAt", "")
                if closed_at:
                    ts = datetime.fromisoformat(closed_at.replace("Z", "+00:00")).timestamp()
                    if ts >= cutoff_ts:
                        closed_issues += 1
        except Exception:
            pass

    return {"merged_recently": merged, "closed_recently": closed_issues}


def parse_pr_detail(detail) -> dict:
    """Parse a detailed PR response into a clean status dict."""
    # Reviews grouped by author (latest state per author)
    reviews_by_author: dict[str, str] = {}
    for review in detail.get("reviews", []) or []:
        author = review.get("author", {}).get("login", "unknown")
        state = review.get("state", "")
        if state:  # keep latest
            reviews_by_author[author] = state

    # CI checks
    checks = []
    for check in detail.get("statusCheckRollup", []) or []:
        name = check.get("name") or check.get("context", "unknown")
        conclusion = check.get("conclusion", "")
        status = check.get("status", "")
        if conclusion:
            state = conclusion.upper()
        elif status:
            state = status.upper()
        else:
            state = "PENDING"
        checks.append({"name": name, "state": state})

    # Human comments (filter bots)
    human_comments = []
    for comment in detail.get("comments", []) or []:
        author = comment.get("author", {}).get("login", "")
        if author in BOT_LOGINS:
            continue
        human_comments.append({
            "author": author,
            "createdAt": comment.get("createdAt", ""),
            "body": (comment.get("body", "") or "")[:200],
        })

    labels = [l.get("name", "") for l in detail.get("labels", []) or []]
    author = detail.get("author", {}).get("login", "unknown")

    checks_pass = sum(1 for c in checks if c["state"] == "SUCCESS")
    checks_fail = sum(1 for c in checks if c["state"] == "FAILURE")
    checks_pending = sum(1 for c in checks if c["state"] in ("PENDING", "IN_PROGRESS", "QUEUED"))

    approved = sum(1 for s in reviews_by_author.values() if s == "APPROVED")
    changes_requested = sum(1 for s in reviews_by_author.values() if s == "CHANGES_REQUESTED")

    last_human = None
    if human_comments:
        lc = human_comments[-1]
        last_human = {
            "author": lc["author"],
            "body": lc["body"],
            "time": lc["createdAt"],
        }

    return {
        "number": detail.get("number"),
        "title": detail.get("title", ""),
        "author": author,
        "status": {
            "state": detail.get("state", ""),
            "isDraft": detail.get("isDraft", False),
            "head": detail.get("headRefName", ""),
            "base": detail.get("baseRefName", ""),
            "labels": labels,
            "updated": detail.get("updatedAt", ""),
            "mergeable": detail.get("mergeable", ""),
            "mergeStateStatus": detail.get("mergeStateStatus", ""),
            "reviewers": reviews_by_author,
            "approved": approved,
            "changes_requested": changes_requested,
            "checks_pass": checks_pass,
            "checks_fail": checks_fail,
            "checks_pending": checks_pending,
            "checks_summary": _summarize_checks(checks),
            "human_comments": len(human_comments),
            "last_human_comment": last_human,
        },
    }


def _summarize_checks(checks: list[dict]) -> str:
    if not checks:
        return "none"
    states = [c["state"] for c in checks]
    if all(s == "SUCCESS" for s in states):
        return "pass"
    if any(s == "FAILURE" for s in states):
        return "fail"
    if any(s in ("PENDING", "IN_PROGRESS", "QUEUED") for s in states):
        return "pending"
    return "mixed"


def parse_issue(issue: dict) -> dict:
    """Parse an issue into a clean dict."""
    labels = [l.get("name", "") for l in issue.get("labels", []) or []]
    author = issue.get("author", {}).get("login", "unknown")
    assignees = [a.get("login", "") for a in issue.get("assignees", []) or []]
    created = issue.get("createdAt", "")
    age_days = 0
    if created:
        try:
            ts = datetime.fromisoformat(created.replace("Z", "+00:00"))
            age_days = (datetime.now(timezone.utc) - ts).days
        except Exception:
            pass

    milestone = None
    ms = issue.get("milestone")
    if ms:
        milestone = ms.get("title", "")

    return {
        "number": issue.get("number"),
        "title": issue.get("title", ""),
        "author": author,
        "labels": labels,
        "assignees": assignees,
        "createdAt": created,
        "updatedAt": issue.get("updatedAt", ""),
        "age_days": age_days,
        "milestone": milestone,
    }


def fetch_latest_release() -> dict:
    """Fetch the latest release tag and date."""
    try:
        raw = gh(["api", f"repos/{REPO}/releases/latest"])
        data = json.loads(raw)
        return {
            "tag": data.get("tag_name", ""),
            "name": data.get("name", ""),
            "published_at": data.get("published_at", ""),
            "html_url": data.get("html_url", ""),
        }
    except Exception:
        return {}


def fetch_active_branches() -> list:
    """Fetch branches with recent activity (non-default)."""
    try:
        raw = gh(["api", f"repos/{REPO}/branches", "--paginate", "-q",
                  '.[].name'], timeout=15)
        branches = [b.strip() for b in raw.splitlines() if b.strip() and b.strip() != "main"]
        return branches[:20]  # cap at 20
    except Exception:
        return []


def parse_milestone(ms: dict) -> dict:
    """Parse a milestone from the API response."""
    return {
        "title": ms.get("title", ""),
        "state": ms.get("state", ""),
        "open_issues": ms.get("open_issues", 0),
        "closed_issues": ms.get("closed_issues", 0),
        "due_on": ms.get("due_on"),
        "description": (ms.get("description") or "")[:200],
    }


# ---------------------------------------------------------------------------
# Main data assembly
# ---------------------------------------------------------------------------

def build_status() -> dict:
    """Fetch all data and return the full status payload."""
    global _cache, _cache_ts

    with _cache_lock:
        if _cache_valid() and _cache:
            return _cache

    t0 = time.time()
    log(f"{C_CYAN}Fetching data from GitHub...{C_RESET}")

    # Fetch PR list and issues in parallel
    pr_list = []
    issues_raw = []
    milestones_raw = []
    recent = {}

    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as pool:
        f_prs = pool.submit(fetch_pr_list)
        f_issues = pool.submit(fetch_issues)
        f_milestones = pool.submit(fetch_milestones)
        f_recent = pool.submit(fetch_recent_closed)
        f_release = pool.submit(fetch_latest_release)
        f_branches = pool.submit(fetch_active_branches)

        pr_list = f_prs.result()
        issues_raw = f_issues.result()
        milestones_raw = f_milestones.result()
        recent = f_recent.result()
        latest_release = f_release.result()
        branches = f_branches.result()

    # Fetch detailed PR status in parallel
    pr_statuses = []
    if pr_list:
        with ThreadPoolExecutor(max_workers=MAX_WORKERS) as pool:
            futures = {
                pool.submit(fetch_pr_detail, pr["number"]): pr["number"]
                for pr in pr_list
            }
            for future in as_completed(futures):
                num = futures[future]
                try:
                    detail = future.result()
                    pr_statuses.append(parse_pr_detail(detail))
                except Exception as e:
                    log(f"{C_RED}Failed to fetch PR #{num}: {e}{C_RESET}")

    pr_statuses.sort(key=lambda p: p["number"], reverse=True)

    issues = [parse_issue(i) for i in issues_raw]
    issues.sort(key=lambda i: i["number"], reverse=True)

    milestones = [parse_milestone(ms) for ms in milestones_raw if ms.get("state") == "open"]

    stats = {
        "open_issues": len(issues),
        "open_prs": len(pr_statuses),
        "merged_30d": recent.get("merged_recently", 0),
        "closed_30d": recent.get("closed_recently", 0),
    }

    result = {
        "prs": pr_statuses,
        "issues": issues,
        "stats": stats,
        "milestones": milestones,
        "latest_release": latest_release,
        "branches": branches,
        "time": datetime.now(timezone.utc).isoformat(),
        "fetch_time_ms": int((time.time() - t0) * 1000),
    }

    with _cache_lock:
        _cache = result
        _cache_ts = time.time()

    log(
        f"{C_GREEN}Fetched {len(pr_statuses)} PRs, {len(issues)} issues "
        f"in {result['fetch_time_ms']}ms{C_RESET}"
    )
    return result


# ---------------------------------------------------------------------------
# HTTP server
# ---------------------------------------------------------------------------

class CommandCenterHandler(SimpleHTTPRequestHandler):
    """Serves the dashboard HTML and /api/status endpoint."""

    def do_GET(self):
        if self.path == "/api/status":
            self._serve_status()
        elif self.path in ("/", "/index.html"):
            self._serve_html()
        else:
            self.send_error(404)

    def _serve_status(self):
        try:
            data = build_status()
            body = json.dumps(data, indent=2).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Access-Control-Allow-Origin", "*")
            self.end_headers()
            self.wfile.write(body)
        except Exception as e:
            body = json.dumps({"error": str(e)}).encode()
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    def _serve_html(self):
        html_path = Path(__file__).parent / "command_center.html"
        if not html_path.exists():
            body = FALLBACK_HTML.encode()
        else:
            body = html_path.read_bytes()
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):  # noqa: A002
        pass


FALLBACK_HTML = """<!DOCTYPE html>
<html>
<head><title>Mnemonic Command Center</title></head>
<body style="font-family:monospace;background:#1a1a2e;color:#e0e0e0;padding:2em">
<h1>Mnemonic Command Center</h1>
<p>Place <code>command_center.html</code> in the same directory as this script for the full dashboard.</p>
<p>API endpoint: <a href="/api/status" style="color:#00d4ff">/api/status</a></p>
<pre id="data" style="background:#16213e;padding:1em;border-radius:4px;overflow:auto;max-height:80vh"></pre>
<script>
fetch('/api/status').then(r=>r.json()).then(d=>{
  document.getElementById('data').textContent=JSON.stringify(d,null,2);
}).catch(e=>{document.getElementById('data').textContent='Error: '+e;});
setInterval(()=>{
  fetch('/api/status').then(r=>r.json()).then(d=>{
    document.getElementById('data').textContent=JSON.stringify(d,null,2);
  });
}, 30000);
</script>
</body>
</html>"""


# ---------------------------------------------------------------------------
# Terminal output
# ---------------------------------------------------------------------------

def log(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"{C_DIM}[{ts}]{C_RESET} {msg}")


def print_banner(port: int):
    print(f"""
{C_BOLD}{C_CYAN}{'=' * 52}
  Mnemonic Command Center
  Repo: {REPO}
{'=' * 52}{C_RESET}

  {C_GREEN}Dashboard:{C_RESET}  http://localhost:{port}/
  {C_GREEN}API:{C_RESET}        http://localhost:{port}/api/status
  {C_GREEN}Cache TTL:{C_RESET}  {CACHE_TTL}s
  {C_GREEN}Workers:{C_RESET}    {MAX_WORKERS}

  {C_DIM}Press Ctrl+C to stop{C_RESET}
""")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Mnemonic Command Center")
    parser.add_argument("--port", type=int, default=8042, help="Port to serve on (default: 8042)")
    parser.add_argument("--no-browser", action="store_true", help="Don't auto-open browser")
    args = parser.parse_args()

    # Verify gh is available
    try:
        subprocess.run(["gh", "auth", "status"], capture_output=True, check=True, timeout=10)
    except FileNotFoundError:
        print(f"{C_RED}Error: gh CLI not found. Install from https://cli.github.com/{C_RESET}")
        sys.exit(1)
    except subprocess.CalledProcessError:
        print(f"{C_RED}Error: gh CLI not authenticated. Run 'gh auth login' first.{C_RESET}")
        sys.exit(1)

    print_banner(args.port)

    server = HTTPServer(("0.0.0.0", args.port), CommandCenterHandler)

    if not args.no_browser:
        threading.Timer(1.0, lambda: webbrowser.open(f"http://localhost:{args.port}/")).start()

    try:
        log(f"Listening on port {C_BOLD}{args.port}{C_RESET}")
        server.serve_forever()
    except KeyboardInterrupt:
        log(f"{C_YELLOW}Shutting down...{C_RESET}")
        server.shutdown()


if __name__ == "__main__":
    main()
