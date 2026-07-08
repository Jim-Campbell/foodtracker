// Package docs embeds the diet-quality reference documents so the AI parser
// can include them verbatim in its system prompt (never paraphrased).
package docs

import _ "embed"

//go:embed diet-framework.md
var DietFramework string
