# recommit

Reshape all commits in the current branch. Uncommits all changes back to local changes, then creates new atomic commits following make_commit rules.

## Commit Format

```
<branch-name>: Short description in English

More detailed description of the changes.
```

Branch name is determined via `git branch --show-current`.

## Instructions

1. **Find branch divergence point**:
   - Get current branch name via `git branch --show-current`
   - Find commits not in main: `git log --oneline --first-parent main..HEAD`
   - Count determines the base point: `HEAD~<count>`
2. **CRITICAL — Confirm before uncommit**:
   - Get commit list via `git log --oneline --first-parent main..HEAD`
   - **Show the user the full list of commits to be removed**:
     ```
     The following commits will be removed:
     <hash1> <message1>
     <hash2> <message2>
     ...

     Total: <N> commits
     ```
   - **Ask for user confirmation**
   - Proceed only if the user confirms
   - **If the user declines — STOP immediately**
3. **Preserve changes**: Run `git reset --soft HEAD~<count>` to uncommit but keep changes staged
4. **Unstage changes**: Run `git reset HEAD` to move all changes to unstaged
5. **Analyze changes**: Review all modified and new files via `git status` and `git diff`
6. **Group by context**: Split changes into logical groups
7. **Create atomic commits**: Each commit should contain only related changes from one context
8. **Prefix**: Use current branch name as commit prefix
9. **Subject line**: Short description (up to 72 chars)
10. **Commit body**: Detailed description

## Rules

- **Warning**: This command rewrites commit history!
- One commit = one context of changes
- Do not mix refactoring with new features
- Do not include unrelated files in one commit
- Descriptions in English
- Uses `git log --first-parent main..HEAD` to determine branch commits

## Post-commit cleanup

After creating all new atomic commits:

1. **Verify created commits**:
   ```bash
   git log main..HEAD --format="%H%n%B%n---"
   ```

2. **Find and remove Co-Authored-By** if present:
   ```bash
   git commit --amend -m "$(git log -1 --format=%B | sed '/^Co-Authored-By:/,$d')"
   ```

**Critical:**
- Cleanup happens AFTER all atomic commits are created
- Check ALL commits in the branch (relative to main)
- Remove the entire footer starting from "Co-Authored-By:"
