---
name: release
description: Analyze changes, bump version, merge to main if needed, commit, tag, and push a new release.
---

# Release Command

Full release workflow: analyze changes, bump version, handle merges, tag, and push. This skill must work regardless of the current project state (dirty tree, any branch, behind remote, etc.).

## Workflow

### Step 0: Resolve project state

Run these commands first to understand the full picture:

```bash
git status
git branch --show-current
git fetch origin
git log --oneline -1 origin/main  # is remote ahead?
```

**Dirty working tree?** If there are uncommitted changes:
- Ask the user: include them in the release, stash them, or abort.
- If "include": stage and commit them first with a descriptive message before proceeding.
- If "stash": `git stash push -m "pre-release stash"` and pop after release.
- If "abort": stop.

**Behind remote?** If the current branch is behind its upstream:
- Run `git pull --rebase` first. If conflicts arise, stop and tell the user to resolve them manually.

### Step 1: Branch check & merge to main

The release tag must live on `main`.

**If on `main`:**
- Confirm with the user before proceeding. Releasing from main is the final step.

**If on a different branch (e.g., `dev`):**
1. First, ensure `main` is up to date: `git checkout main && git pull origin main`
2. Merge the source branch into main: `git merge <source-branch>`
   - If merge conflicts: STOP. Tell the user to resolve conflicts manually in the working tree, then re-run `/release`.
   - If the merge succeeds, continue from `main`.
3. Note the source branch name so you can return to it or mention it.

**If merge conflicts at any point:** Abort the merge (`git merge --abort`) if applicable, and tell the user exactly which files conflict. Do NOT proceed with conflicts.

### Step 2: Analyze changes

```bash
git log --oneline --no-merges $(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")..HEAD
```

Present the user with a summary:
- Commits since last tag
- Type of changes detected (bug fix, feature, docs, refactor)

### Step 3: Determine version bump

Read the current version from `upgopher.go` (the `var version =` line).

Based on the analyzed changes, recommend:
- **Patch** (X.Y.Z → X.Y.Z+1): bug fixes, docs, refactors, minor tweaks.
- **Minor** (X.Y.Z → X.Y+1.0): new features, new endpoints, new UI, new flags.

Ask the user to confirm or override the bump type. Do NOT proceed until the user confirms.

### Step 4: Update version

Edit `upgopher.go` and change the `version` variable to the new version string.

Example: `var version = "1.16.0"` → `var version = "1.17.0"`

### Step 5: Commit the version bump

```bash
git add upgopher.go
git commit -m "chore: bump version to X.Y.Z"
```

If other files were already staged before this skill ran, include them too.

### Step 6: Create tag

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
```

The tag name is `v` + the version string. Verify it doesn't already exist (`git tag -l vX.Y.Z`).

### Step 7: Push

```bash
git push origin main
git push origin vX.Y.Z
```

If the release was done from a feature branch that was merged to main, also push the source branch if it had unpushed commits.

### Step 8: Cleanup

If changes were stashed in Step 0, pop them now:
```bash
git checkout <original-branch>
git stash pop
```

Show a final summary: new version, tag name, branch pushed to.

## Important rules

- **Never skip conflict checks.** Any merge or rebase conflict is a hard stop.
- **Always confirm with the user** before: pushing to main, merging branches, or creating tags.
- **Version in `upgopher.go` and git tag must match**: version `1.17.0` → tag `v1.17.0`.
- **Do NOT amend commits.** Always create new ones.
- **Do NOT force push.** If push is rejected, investigate why instead of forcing.
- **Work regardless of project state** — handle dirty trees, detached HEAD, behind/ahead of remote, etc.
