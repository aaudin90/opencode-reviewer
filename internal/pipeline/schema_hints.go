package pipeline

const reviewSchemaHint = `{
  "reviewer_name": "string",
  "summary": "string",
  "verdict": "approve|request_changes|comment_only|skipped",
  "findings": [{"file": "string", "start_line": 1, "end_line": 2, "existing_code": "string", "confidence": "high|medium|low", "issue_content": "string", "recommendation": "string"}]
}`

const finalizerSchemaHint = `{
  "summary": "string",
  "verdict": "approve|request_changes|comment_only",
  "findings": [{"file": "string", "start_line": 1, "end_line": 2, "existing_code": "string", "confidence": "high|medium|low", "issue_content": "string", "recommendation": "string", "sources": ["string"]}]
}`
