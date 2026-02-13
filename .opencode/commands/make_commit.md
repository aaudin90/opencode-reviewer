# make_commit

Create atomic git commits from current changes.

## Commit Format

```
<branch-name>: Short description in English

More detailed description of the changes.
```

Branch name is determined via `git branch --show-current`.

## Instructions

1. **Determine prefix**: Get current branch name via `git branch --show-current` — this is the commit prefix
2. **Analyze changes**: Review all modified and new files via `git status` and `git diff`
3. **Group by context**: Split changes into logical groups (e.g.: refactoring, new feature, bug fix, dependency update)
4. **Create atomic commits**: Each commit should contain only related changes from one context
5. **Subject line**: Short description (up to 72 chars) reflecting the essence of changes
6. **Commit body**: Detailed description:
   - What was changed
   - Why it was done (if not obvious)
   - Any side effects or important details

## Example

If the current branch is `78288`:

```
78288: add config validation on load

Added required field validation for configuration.
Return clear error when endpoint is missing.
```

## Post-commit cleanup

After creating all commits:

1. **Check recent commits**:
   ```bash
   git log -5 --format="%H%n%B%n---"
   ```

2. **Find commits with Co-Authored-By**:
   - Check each commit for lines starting with "Co-Authored-By:"
   - If found — must be cleaned

3. **Remove Co-Authored-By from commits**:
   ```bash
   # For the last commit
   git commit --amend -m "$(git log -1 --format=%B | sed '/^Co-Authored-By:/,$d')"

   # For multiple commits
   git filter-branch -f --msg-filter 'sed "/^Co-Authored-By:/d"' HEAD~3..HEAD
   ```

4. **Cleanup algorithm**:

   **Single commit (last one):**
   - Get original message: `git log -1 --format=%B`
   - Remove all lines from "Co-Authored-By:" to end: regex `/^Co-Authored-By:/,$d`
   - Apply cleaned message: `git commit --amend`

   **Multiple commits (batch cleanup):**
   - Determine commit range (e.g., HEAD~3..HEAD)
   - Use `git filter-branch` with msg-filter
   - Verify result via `git log`

**Critical:**
- Cleanup must happen AFTER all commits are created
- If "Co-Authored-By:" is found — MUST be removed
- Do not leave trailing blank lines in commit messages

## Rules

- One commit = one context of changes
- Do not mix refactoring with new features
- Do not include unrelated files in one commit
- Descriptions in English
- Follow Git Convention: subject should complete the phrase "This commit..."
