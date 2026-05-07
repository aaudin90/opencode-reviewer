package reviewruntime

import _ "embed"

//go:embed tools/submit_review.ts
var submitReviewTS []byte

//go:embed tools/submit_final_review.ts
var submitFinalReviewTS []byte
