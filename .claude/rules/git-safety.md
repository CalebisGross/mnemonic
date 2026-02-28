# Git Safety

## Branch Workflow

- Remote: `origin` (https://github.com/CalebisGross/mnemonic.git)
- Primary branch: `main`
- **All new work starts on a feature branch** — never commit directly to `main`
- Branch naming: `feat/<description>`, `fix/<description>`
- Before branching: `git stash` (if dirty), `git pull origin main`, then `git checkout -b <branch>`
- **All changes go through a PR** — push the branch, open a PR with `gh pr create`, get it reviewed
- No blind commits to main, no YOLO pushes

## Forbidden Operations

Enforced by `.claude/hooks/protect-git.sh` and `.claude/hooks/no-secrets.sh`:

- `git push --force` / `git push -f` -- destroys remote history
- `git reset --hard` -- destroys local changes
- `git clean -f` -- permanently deletes untracked files
- `git checkout .` / `git restore .` -- discards all unstaged changes
- Staging `.env`, `credentials`, `*.db`, `settings.local.json`

## Commit Messages

- Short, direct subject line describing the change
- Body for context when non-obvious
- No issue-closing keywords in commit messages unless explicitly asked
- Use Co-Authored-By for Claude contributions

## Secrets

- `settings.local.json` contains machine-specific permissions -- NEVER commit
- `*.db` files contain user data -- gitignored
- Never include API tokens in commit messages or code
