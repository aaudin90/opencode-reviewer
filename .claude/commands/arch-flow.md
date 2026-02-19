# arch-flow

Read the source code and regenerate the architecture flow diagram, saving it to `agentdocs/architecture-flow.md`.

## Instructions

1. **Discover source files** using Glob:
   - `cmd/**/*.go`
   - `internal/**/*.go`
   - Exclude `*_test.go` files

2. **Read all discovered files** using the Read tool.

3. **Generate a complete flow diagram** based on the actual current code, covering:
   - CLI entry point: flag parsing, config loading, branch/provider/agent config resolution with priority order
   - Workspace setup: temp directory structure, opencode.json contents, agent file layout
   - Pipeline stages in order with functions called and data flowing between them
   - Runner HTTP API: endpoints, request/response shapes, timeouts
   - Diff pipeline: parse → filter → sort → WriteContextFile
   - Key invariants (isolation, read-only permissions, clean git state, etc.)

4. **Output the result directly in the chat** as your response text. Do NOT write to any file.

## Formatting rules

Use this exact visual style throughout:

- Section headers as Unicode boxes:
  ```
  ╔══════════════════════════════════╗
  ║  SECTION NAME                    ║
  ╚══════════════════════════════════╝
  ```

- Tree structure with Unicode box-drawing characters:
  ```
  parent
  │
  ├─ item A
  │   ├─ sub-item
  │   └─ sub-item
  │
  └─ item B
  ```

- Inline comments with `←` and `→` arrows
- Actual request/response JSON bodies shown inline in the tree
- File paths shown as indented directory trees inside the flow
- Key invariants section at the end with `✓` checkmarks
- Write in Russian (labels, comments, section names, descriptions)
- No fenced code blocks — the diagram is the document content itself
