# Code Quality & Scope Discipline

## Finish the Work First

- The goal is working, tested code — not a PR. PRs are the delivery mechanism, not the deliverable.
- Do NOT rush to commit, push, or open a PR until the actual work is done and verified.
- A task is not done when the code compiles. It's done when it runs correctly and is tested.
- Stay in the problem. If something is "working," verify it properly before moving to git/PR mechanics.
- Only create a PR when explicitly asked, or when the work is genuinely complete and tested.
- Never treat "open a PR" as a natural next step — wait for the user to decide when the work is ready.

## Scope

- Only change what was asked for — don't touch surrounding code
- If you spot something worth fixing but it wasn't requested, call it out instead of silently doing it
- No drive-by refactors, no "while I'm here" improvements
- One logical change per task — don't bundle unrelated fixes

## Change Safety

- Read before edit — always understand a file before modifying it
- Build and test after changes, don't assume it works
- No new dependencies without discussing it first
- Don't delete code you don't fully understand

## Review Mindset

- Don't add comments, docstrings, or type annotations to untouched code
- Don't rename things that aren't part of the task
- Don't "improve" error messages or formatting in adjacent code
- Keep PRs reviewable — small, focused diffs
- If a change is getting large, pause and check in with the user
