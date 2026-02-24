import { tool } from "@opencode-ai/plugin"

export default tool({
  description: `Submit the consolidated final code review.
Call this tool exactly once after deduplicating and merging all reviewer findings.
Do NOT write findings as plain text.`,

  args: {
    summary: tool.schema.string()
      .describe("Overall 2-4 sentence assessment across all reviewers"),
    verdict: tool.schema.enum(["approve", "request_changes", "comment_only"]),
    findings: tool.schema.array(tool.schema.object({
      file:           tool.schema.string(),
      start_line:     tool.schema.number().int(),
      end_line:       tool.schema.number().int(),
      existing_code:  tool.schema.string(),
      confidence:     tool.schema.enum(["high", "medium", "low"]),
      issue_content:  tool.schema.string(),
      recommendation: tool.schema.string(),
      sources:        tool.schema.array(tool.schema.string())
                        .describe("reviewer_names that identified this finding"),
    })).describe("Deduplicated merged findings. Empty array [] if no issues."),
  },

  async execute(args, _context) {
    return `Final review submitted: ${args.findings.length} finding(s), verdict: ${args.verdict}.`
  },
})
