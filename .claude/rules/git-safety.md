# Git Safety

## Branch Workflow

- Remote: `origin` (https://github.com/CalebisGross/mnemonic.git)
- Primary branch: `main`
- **All new work starts on a feature branch** — never commit directly to `main`
- Branch naming: `feat/<description>`, `fix/<description>`
- Before branching: `git stash` (if dirty), `git pull origin main`, then `git checkout -b <branch>`
- **All changes go through a PR** — push the branch, open a PR with `gh pr create`, get it reviewed
- **Closing issues:** When a PR resolves a GitHub issue, comment on the issue with a reference to the PR before or after closing it. Never close issues silently.
- No blind commits to main, no YOLO pushes

## Forbidden Operations

Enforced by `.claude/hooks/protect-git.sh` and `.claude/hooks/no-secrets.sh`:

- `git push --force` / `git push -f` -- destroys remote history
- `git reset --hard` -- destroys local changes
- `git clean -f` -- permanently deletes untracked files
- `git checkout .` / `git restore .` -- discards all unstaged changes
- Staging `.env`, `credentials`, `*.db`, `settings.local.json`

## Commit Messages (Conventional Commits)

Use [Conventional Commits](https://www.conventionalcommits.org/) format — release-please uses these to auto-generate changelogs and version bumps:

- `feat: add memory source tracking` — new feature (bumps minor)
- `fix: prevent nil pointer in retrieval` — bug fix (bumps patch)
- `docs: update README with Gemini setup` — documentation only
- `refactor: simplify consolidation loop` — code change, no behavior change
- `test: add encoding agent coverage` — tests only
- `chore: update dependencies` — maintenance
- `ci: fix release workflow runner` — CI/CD changes

Rules:

- Short, direct subject line describing the change
- Body for context when non-obvious
- No issue-closing keywords in commit messages unless explicitly asked
- Use Co-Authored-By for Claude contributions
- Append `!` after the type for breaking changes: `feat!: redesign store interface`

## Secrets

- `settings.local.json` contains machine-specific permissions -- NEVER commit
- `*.db` files contain user data -- gitignored
- Never include API tokens in commit messages or code
