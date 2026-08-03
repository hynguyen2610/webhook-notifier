package main

import (
	"fmt"
	"html"
	"strings"
	"time"
)

func buildHTMLReport(reportTime time.Time, benchmarkOptions benchmarkOptions, schedulerSummaries []benchmarkSummary, fairnessScenarioSummaries []fairnessScenarioSummary) string {
	var builder strings.Builder
	maxNsPerOp := int64(1)
	maxAllocsPerOp := int64(1)
	maxBytesPerOp := int64(1)

	for _, summary := range schedulerSummaries {
		if summary.nsPerOp > maxNsPerOp {
			maxNsPerOp = summary.nsPerOp
		}
		if summary.allocsPerOp > maxAllocsPerOp {
			maxAllocsPerOp = summary.allocsPerOp
		}
		if summary.bytesPerOp > maxBytesPerOp {
			maxBytesPerOp = summary.bytesPerOp
		}
	}

	builder.WriteString("<!DOCTYPE html>\n")
	builder.WriteString("<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	builder.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	builder.WriteString("<title>Scheduler Benchmark Report</title>\n")
	builder.WriteString("<style>\n")
	builder.WriteString("body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,\"Segoe UI\",sans-serif;background:#f4efe7;color:#1f2933;}\n")
	builder.WriteString(".page{max-width:1100px;margin:0 auto;padding:40px 24px 56px;}\n")
	builder.WriteString(".hero{background:linear-gradient(135deg,#133c55,#386fa4);color:#fff;border-radius:20px;padding:28px 32px;box-shadow:0 18px 40px rgba(19,60,85,.22);}\n")
	builder.WriteString(".hero h1{margin:0 0 8px;font-size:34px;line-height:1.1;}\n")
	builder.WriteString(".hero p{margin:8px 0 0;max-width:760px;font-size:15px;line-height:1.6;color:rgba(255,255,255,.88);}\n")
	builder.WriteString(".meta{margin-top:14px;font-size:13px;color:rgba(255,255,255,.76);}\n")
	builder.WriteString(".section{margin-top:28px;}\n")
	builder.WriteString(".section h2{margin:0 0 14px;font-size:20px;color:#102a43;}\n")
	builder.WriteString(".section-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:18px;}\n")
	builder.WriteString(".tabs{display:flex;gap:10px;flex-wrap:wrap;margin-top:28px;}\n")
	builder.WriteString(".tab-button{border:none;border-radius:999px;padding:12px 18px;font-size:14px;font-weight:600;background:#d9e2ec;color:#243b53;cursor:pointer;transition:all .2s ease;}\n")
	builder.WriteString(".tab-button.active{background:#133c55;color:#fff;box-shadow:0 10px 24px rgba(19,60,85,.18);}\n")
	builder.WriteString(".tab-panel{display:none;}\n")
	builder.WriteString(".tab-panel.active{display:block;}\n")
	builder.WriteString(".table-wrap{background:#fff;border-radius:18px;padding:16px 16px 8px;box-shadow:0 14px 30px rgba(16,42,67,.08);overflow:auto;}\n")
	builder.WriteString(".info-card{background:#fff;border-radius:18px;padding:18px;box-shadow:0 14px 30px rgba(16,42,67,.08);}\n")
	builder.WriteString(".info-card h3{margin:0 0 12px;font-size:16px;color:#102a43;}\n")
	builder.WriteString(".pill{display:inline-flex;align-items:center;border-radius:999px;padding:6px 12px;font-size:12px;font-weight:700;letter-spacing:.03em;text-transform:uppercase;background:#fde68a;color:#7c2d12;}\n")
	builder.WriteString(".callout{margin-top:16px;background:#fff7ed;border-left:4px solid #d9822b;border-radius:14px;padding:14px 16px;color:#7c2d12;box-shadow:0 14px 30px rgba(16,42,67,.05);}\n")
	builder.WriteString("table{width:100%;border-collapse:collapse;font-size:14px;}\n")
	builder.WriteString("th,td{padding:12px 14px;border-bottom:1px solid #e6ecf1;}\n")
	builder.WriteString("th{text-align:left;background:#f8fafc;color:#486581;font-size:12px;letter-spacing:.04em;text-transform:uppercase;}\n")
	builder.WriteString("td.num,th.num{text-align:right;font-variant-numeric:tabular-nums;}\n")
	builder.WriteString("tr:last-child td{border-bottom:none;}\n")
	builder.WriteString(".charts{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:18px;}\n")
	builder.WriteString(".chart-card{background:#fff;border-radius:18px;padding:18px;box-shadow:0 14px 30px rgba(16,42,67,.08);}\n")
	builder.WriteString(".chart-card h3{margin:0 0 14px;font-size:16px;color:#102a43;}\n")
	builder.WriteString(".bar-list{display:flex;flex-direction:column;gap:12px;}\n")
	builder.WriteString(".bar-row{display:grid;grid-template-columns:170px 1fr 92px;gap:12px;align-items:center;}\n")
	builder.WriteString(".bar-label{font-size:13px;color:#334e68;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}\n")
	builder.WriteString(".bar-track{height:14px;background:#e9eff5;border-radius:999px;overflow:hidden;}\n")
	builder.WriteString(".bar-fill{height:100%;border-radius:999px;background:linear-gradient(90deg,#d9822b,#f0b429);}\n")
	builder.WriteString(".bar-fill.alt{background:linear-gradient(90deg,#2f855a,#48bb78);}\n")
	builder.WriteString(".bar-fill.cool{background:linear-gradient(90deg,#2b6cb0,#63b3ed);}\n")
	builder.WriteString(".bar-value{text-align:right;font-size:12px;color:#486581;font-variant-numeric:tabular-nums;}\n")
	builder.WriteString(".footnote{margin-top:16px;font-size:13px;color:#627d98;}\n")
	builder.WriteString("@media (max-width:720px){.bar-row{grid-template-columns:1fr;}.bar-value{text-align:left;}.tabs{flex-direction:column;}.tab-button{width:100%;}}\n")
	builder.WriteString("</style>\n</head>\n<body>\n<div class=\"page\">\n")
	builder.WriteString("<section class=\"hero\">\n")
	builder.WriteString("<h1>Scheduler Benchmark Report</h1>\n")
	builder.WriteString("<p>This report separates scheduler throughput evidence from fairness evidence so reviewers can quickly tell what was measured, what the numbers mean, and how much app-level confidence they should take from the run.</p>\n")
	builder.WriteString(fmt.Sprintf("<div class=\"meta\">Generated at %s UTC</div>\n", reportTime.Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("<div class=\"meta\">Mode: %s</div>\n", benchmarkOptions.mode))
	if !benchmarkOptions.includeLargeScenario {
		builder.WriteString("<div class=\"meta\"><span class=\"pill\">Large fairness scenario skipped</span></div>\n")
	}
	builder.WriteString("</section>\n")
	builder.WriteString(buildHTMLIntroSections(benchmarkOptions))
	builder.WriteString("<section class=\"tabs\" aria-label=\"Benchmark report sections\">\n")
	builder.WriteString("<button class=\"tab-button active\" type=\"button\" data-tab-target=\"throughput-tab\">Throughput Benchmark</button>\n")
	builder.WriteString("<button class=\"tab-button\" type=\"button\" data-tab-target=\"fairness-tab\">Fairness Benchmark</button>\n")
	builder.WriteString("</section>\n")
	builder.WriteString("<section class=\"tab-panel active\" id=\"throughput-tab\">\n")
	builder.WriteString("<section class=\"section\">\n<h2>Scheduler Results</h2>\n<div class=\"table-wrap\">\n")
	builder.WriteString("<div class=\"callout\">This tab is scheduler-only evidence. Use it to compare scheduler cost by workload. Do not read these numbers as app-mode throughput or deployment-level scale proof.</div>\n")
	builder.WriteString("<table>\n<thead><tr><th>Scenario</th><th class=\"num\">Jobs / iteration</th><th class=\"num\">ns/op</th><th class=\"num\">allocs/op</th><th class=\"num\">bytes/op</th><th class=\"num\">ops/sec</th><th class=\"num\">jobs/sec</th></tr></thead>\n<tbody>\n")

	for _, summary := range schedulerSummaries {
		builder.WriteString(fmt.Sprintf(
			"<tr><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%.2f</td><td class=\"num\">%.2f</td></tr>\n",
			html.EscapeString(summary.name),
			summary.jobCount,
			summary.nsPerOp,
			summary.allocsPerOp,
			summary.bytesPerOp,
			summary.throughputOps,
			summary.jobsPerSecond,
		))
	}

	builder.WriteString("</tbody>\n</table>\n</div>\n</section>\n")
	builder.WriteString("<section class=\"section\">\n<h2>Charts</h2>\n<div class=\"charts\">\n")
	builder.WriteString(buildHTMLChartCard("Latency by Scenario (ns/op)", schedulerSummaries, maxNsPerOp, "ns", ""))
	builder.WriteString(buildHTMLChartCard("Allocations by Scenario (allocs/op)", schedulerSummaries, maxAllocsPerOp, "allocs", "alt"))
	builder.WriteString(buildHTMLChartCard("Memory by Scenario (bytes/op)", schedulerSummaries, maxBytesPerOp, "bytes", "cool"))
	builder.WriteString("</div>\n<div class=\"footnote\">Longer bars represent larger cost per benchmark iteration. Use these charts to compare how scheduler overhead grows as the workload gets larger.</div>\n</section>\n")
	builder.WriteString("</section>\n")
	builder.WriteString("<section class=\"tab-panel\" id=\"fairness-tab\">\n")
	builder.WriteString(buildHTMLFairnessScenarioSections(benchmarkOptions.mode, fairnessScenarioSummaries))
	builder.WriteString("</section>\n")
	builder.WriteString("<script>\n")
	builder.WriteString("const tabButtons=document.querySelectorAll('[data-tab-target]');\n")
	builder.WriteString("const tabPanels=document.querySelectorAll('.tab-panel');\n")
	builder.WriteString("tabButtons.forEach((tabButton)=>{tabButton.addEventListener('click',()=>{const targetId=tabButton.getAttribute('data-tab-target');tabButtons.forEach((button)=>button.classList.remove('active'));tabPanels.forEach((panel)=>panel.classList.remove('active'));tabButton.classList.add('active');document.getElementById(targetId).classList.add('active');});});\n")
	builder.WriteString("</script>\n")
	builder.WriteString("</div>\n</body>\n</html>\n")

	return builder.String()
}

func buildHTMLFairnessScenarioSections(mode benchmarkMode, fairnessScenarioSummaries []fairnessScenarioSummary) string {
	var builder strings.Builder

	builder.WriteString("<section class=\"section\">\n")
	builder.WriteString(fmt.Sprintf("<h2>Worker Fairness Scenarios (%s mode)</h2>\n", html.EscapeString(string(mode))))
	if mode == benchmarkModeApp {
		builder.WriteString("<p>App mode covers the in-memory notifier fairness pipeline: enqueue, queue claim, scheduler handoff, worker execution, and synthetic completion timing. It does not include PostgreSQL queue behavior or real webhook network delivery.</p>\n")
	} else {
		builder.WriteString("<p>Scheduler mode covers the scheduler plus a synthetic worker harness. It is useful for fairness shape, but it is not deployment-level app evidence.</p>\n")
	}
	builder.WriteString("</section>\n")

	for _, scenarioSummary := range fairnessScenarioSummaries {
		builder.WriteString("<section class=\"section\">\n")
		builder.WriteString(fmt.Sprintf("<h2>%s</h2>\n", html.EscapeString(scenarioSummary.name)))
		builder.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(scenarioSummary.description)))
		builder.WriteString(buildHTMLFairnessConclusionList(mode, scenarioSummary))
		builder.WriteString("<div class=\"table-wrap\">\n")
		builder.WriteString("<table>\n<thead><tr><th>Workers</th><th class=\"num\">Jobs</th><th class=\"num\">Duration</th><th class=\"num\">jobs/sec</th></tr></thead>\n<tbody>\n")
		for _, runSummary := range scenarioSummary.workerRunSummaries {
			builder.WriteString(fmt.Sprintf(
				"<tr><td>%d</td><td class=\"num\">%d</td><td class=\"num\">%s</td><td class=\"num\">%.2f</td></tr>\n",
				runSummary.workerCount,
				runSummary.totalJobCount,
				runSummary.totalDuration.Round(time.Millisecond),
				runSummary.totalJobsPerSecond,
			))
		}
		builder.WriteString("</tbody>\n</table>\n</div>\n")
		builder.WriteString("<div class=\"table-wrap\" style=\"margin-top:16px;\">\n")
		builder.WriteString("<table>\n<thead><tr><th>Workers</th><th>Segment</th><th class=\"num\">Messages</th><th>Customer</th><th class=\"num\">First completion</th><th class=\"num\">Full completion</th><th class=\"num\">Early completions</th><th class=\"num\">Early-window share</th><th class=\"num\">Whale vs non-whale full gap</th><th>Normals before whales?</th></tr></thead>\n<tbody>\n")
		for _, runSummary := range scenarioSummary.workerRunSummaries {
			runInsights := analyzeFairnessRun(runSummary.customerSummaries)
			nonWhalesFinishedBeforeWhales := "No"
			if runInsights.nonWhalesFinishedBeforeWhales {
				nonWhalesFinishedBeforeWhales = "Yes"
			}
			for _, customerSummary := range runSummary.customerSummaries {
				builder.WriteString(fmt.Sprintf(
					"<tr><td>%d</td><td>%s</td><td class=\"num\">%d</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%d</td><td class=\"num\">%.1f%%</td><td class=\"num\">%s</td><td>%s</td></tr>\n",
					runSummary.workerCount,
					html.EscapeString(customerSummary.customerSegment),
					customerSummary.jobCount,
					html.EscapeString(customerSummary.customerID),
					customerSummary.firstFinishDuration.Round(time.Millisecond),
					customerSummary.finishDuration.Round(time.Millisecond),
					customerSummary.earlyCompletionCount,
					customerSummary.earlyCompletionShare*100,
					html.EscapeString(formatDurationGap(runInsights.whaleVsNonWhaleCompletionGap)),
					nonWhalesFinishedBeforeWhales,
				))
			}
		}
		builder.WriteString("</tbody>\n</table>\n</div>\n")
		builder.WriteString(fmt.Sprintf("<div class=\"footnote\">%s</div>\n", html.EscapeString(scenarioSummary.earlyWindowReason)))
		builder.WriteString("<div class=\"charts\" style=\"margin-top:18px;\">\n")
		builder.WriteString(buildScenarioThroughputChart(scenarioSummary))
		builder.WriteString(buildScenarioFinishChart(scenarioSummary))
		builder.WriteString("</div>\n")
		builder.WriteString("</section>\n")
	}

	return builder.String()
}

func buildScenarioThroughputChart(scenarioSummary fairnessScenarioSummary) string {
	maxJobsPerSecond := 1.0
	for _, runSummary := range scenarioSummary.workerRunSummaries {
		if runSummary.totalJobsPerSecond > maxJobsPerSecond {
			maxJobsPerSecond = runSummary.totalJobsPerSecond
		}
	}

	var builder strings.Builder
	builder.WriteString("<article class=\"chart-card\">\n")
	builder.WriteString("<h3>Throughput By Worker Count</h3>\n")
	builder.WriteString("<div class=\"bar-list\">\n")
	for _, runSummary := range scenarioSummary.workerRunSummaries {
		barWidth := (runSummary.totalJobsPerSecond / maxJobsPerSecond) * 100
		builder.WriteString("<div class=\"bar-row\">\n")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-label\">%d workers</div>\n", runSummary.workerCount))
		builder.WriteString("<div class=\"bar-track\">")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-fill cool\" style=\"width:%.2f%%\"></div>", barWidth))
		builder.WriteString("</div>\n")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-value\">%.2f jobs/sec</div>\n", runSummary.totalJobsPerSecond))
		builder.WriteString("</div>\n")
	}
	builder.WriteString("</div>\n</article>\n")
	return builder.String()
}

func buildScenarioFinishChart(scenarioSummary fairnessScenarioSummary) string {
	maxFinishMilliseconds := 1.0
	for _, runSummary := range scenarioSummary.workerRunSummaries {
		for _, customerSummary := range runSummary.customerSummaries {
			finishMilliseconds := float64(customerSummary.finishDuration.Microseconds()) / 1000
			if finishMilliseconds > maxFinishMilliseconds {
				maxFinishMilliseconds = finishMilliseconds
			}
		}
	}

	var builder strings.Builder
	builder.WriteString("<article class=\"chart-card\">\n")
	builder.WriteString("<h3>Customer Finish Duration</h3>\n")
	builder.WriteString("<div class=\"bar-list\">\n")
	for _, runSummary := range scenarioSummary.workerRunSummaries {
		for _, customerSummary := range runSummary.customerSummaries {
			finishMilliseconds := float64(customerSummary.finishDuration.Microseconds()) / 1000
			barWidth := (finishMilliseconds / maxFinishMilliseconds) * 100
			builder.WriteString("<div class=\"bar-row\">\n")
			builder.WriteString(fmt.Sprintf("<div class=\"bar-label\">%d workers / %s</div>\n", runSummary.workerCount, html.EscapeString(customerSummary.customerID)))
			builder.WriteString("<div class=\"bar-track\">")
			builder.WriteString(fmt.Sprintf("<div class=\"bar-fill alt\" style=\"width:%.2f%%\"></div>", barWidth))
			builder.WriteString("</div>\n")
			builder.WriteString(fmt.Sprintf("<div class=\"bar-value\">%s</div>\n", customerSummary.finishDuration.Round(time.Millisecond)))
			builder.WriteString("</div>\n")
		}
	}
	builder.WriteString("</div>\n</article>\n")
	return builder.String()
}

func buildHTMLChartCard(title string, summaries []benchmarkSummary, maxValue int64, unit string, fillClass string) string {
	var builder strings.Builder

	builder.WriteString("<article class=\"chart-card\">\n")
	builder.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(title)))
	builder.WriteString("<div class=\"bar-list\">\n")

	for _, summary := range summaries {
		var metricValue int64
		switch unit {
		case "allocs":
			metricValue = summary.allocsPerOp
		case "bytes":
			metricValue = summary.bytesPerOp
		default:
			metricValue = summary.nsPerOp
		}

		barWidth := 0.0
		if maxValue > 0 {
			barWidth = (float64(metricValue) / float64(maxValue)) * 100
		}

		builder.WriteString("<div class=\"bar-row\">\n")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-label\">%s</div>\n", html.EscapeString(summary.name)))
		builder.WriteString("<div class=\"bar-track\">")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-fill %s\" style=\"width:%.2f%%\"></div>", fillClass, barWidth))
		builder.WriteString("</div>\n")
		builder.WriteString(fmt.Sprintf("<div class=\"bar-value\">%d %s</div>\n", metricValue, unit))
		builder.WriteString("</div>\n")
	}

	builder.WriteString("</div>\n</article>\n")
	return builder.String()
}
