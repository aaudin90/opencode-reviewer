// @ts-ignore
import { tool } from "@opencode-ai/plugin"

export default tool({
  description: `Submit the final structured code review result.
Call this tool exactly once when your review is complete.
Put ALL findings here. Do NOT write findings as plain text.
If there are no issues, pass an empty findings array with verdict "approve".`,

  args: {
    summary: tool.schema.string().describe("Brief 1-3 sentence overall assessment"),
    verdict: tool.schema.enum(["approve", "request_changes", "comment_only"])
      .describe("approve=no issues; request_changes=has likely bugs or security risks; comment_only=non-blocking findings only"),
    findings: tool.schema.array(tool.schema.object({
      file:           tool.schema.string(),
      start_line:     tool.schema.number().int(),
      end_line:       tool.schema.number().int(),
      existing_code:  tool.schema.string(),
      confidence:     tool.schema.enum(["high", "medium", "low"]),
      issue_content:  tool.schema.string(),
      recommendation: tool.schema.string(),
    })).describe("Empty array [] if no issues found"),
  },

  async execute(args, _context) {
    return `Review submitted: ${args.findings.length} finding(s), verdict: ${args.verdict}.`
  },
})
