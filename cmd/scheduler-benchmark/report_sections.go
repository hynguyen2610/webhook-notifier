package main

import (
	"fmt"
	"html"
	"strings"
)

func buildHTMLIntroSections(benchmarkOptions benchmarkOptions) string {
	var builder strings.Builder

	builder.WriteString("<section class=\"section section-grid\">\n")
	builder.WriteString("<article class=\"info-card\">\n<h2>How To Read This Report</h2>\n<ul>\n")
	for _, line := range buildHowToReadReport(benchmarkOptions.mode) {
		builder.WriteString(fmt.Sprintf("<li>%s</li>\n", html.EscapeString(line)))
	}
	builder.WriteString("</ul>\n</article>\n")
	builder.WriteString("<article class=\"info-card\">\n<h2>Evidence Boundary</h2>\n")
	builder.WriteString("<p>Keep scheduler microbenchmark numbers separate from fairness conclusions. The throughput tab shows component-level scheduler cost. The fairness tab is the right place to judge customer progress behavior.</p>\n")
	if benchmarkOptions.mode == benchmarkModeApp {
		builder.WriteString("<ul>\n")
		for _, line := range appModeConfidenceSummary() {
			builder.WriteString(fmt.Sprintf("<li>%s</li>\n", html.EscapeString(line)))
		}
		builder.WriteString("</ul>\n")
	}
	builder.WriteString("</article>\n")
	builder.WriteString("</section>\n")

	return builder.String()
}

func buildHTMLFairnessConclusionList(mode benchmarkMode, scenarioSummary fairnessScenarioSummary) string {
	var builder strings.Builder
	builder.WriteString("<article class=\"info-card\" style=\"margin-top:16px;\">\n")
	builder.WriteString("<h3>Main Takeaway</h3>\n<ul>\n")
	for _, line := range fairnessConclusionBlock(mode, scenarioSummary) {
		builder.WriteString(fmt.Sprintf("<li>%s</li>\n", html.EscapeString(line)))
	}
	builder.WriteString("</ul>\n</article>\n")
	return builder.String()
}
