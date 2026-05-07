import { tool } from "@opencode-ai/plugin"

export default tool({
  description: `Submit a decision for one GitLab discussion. Call exactly once.`,

  args: {
    action: tool.schema.enum(["reply", "resolve", "unresolve", "noop"]),
    body: tool.schema.string().describe("Required for action=reply. Optional explanatory comment for action=resolve or action=unresolve. Empty for action=noop."),
    confidence: tool.schema.enum(["high", "medium", "low"]),
    would_modify_code: tool.schema.boolean(),
    needs_human: tool.schema.boolean(),
    reason: tool.schema.string(),
  },

  async execute(args, _context) {
    return `Comment warrior decision submitted: ${args.action}.`
  },
})
