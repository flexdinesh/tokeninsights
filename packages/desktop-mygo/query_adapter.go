package main

import (
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
)

func nativeQuery(f filters) analytics.Query {
	period := f.Period
	if period == "custom" {
		period = "all"
	}
	selection := analytics.Selection{Period: period, Bucket: "day", From: f.From, To: f.To}
	if f.Provider != "" {
		selection.Providers = []string{f.Provider}
	}
	if f.Model != "" {
		selection.Models = []string{f.Model}
	}
	if f.Harness != "" {
		selection.Harnesses = []string{f.Harness}
	}
	return analytics.Query{Selection: selection, Tab: "models", Sort: "total", Direction: "desc"}
}

func usageSnapshot(data application.Usage) snapshot {
	summary := data.Summary
	result := snapshot{Range: data.Range, Totals: totals{
		Total: summary.TotalTokens, Input: summary.InputTokens, Output: summary.OutputTokens,
		Reasoning: summary.ReasoningTokens, CacheRead: summary.CacheReadTokens,
		CacheWrite: summary.CacheWriteTokens, Sessions: summary.SessionCount,
	}}
	for _, row := range data.Rows {
		result.Rows = append(result.Rows, usageRow{Provider: row.Provider, Model: row.Model, Harness: row.Harness,
			Tokens: totals{Total: row.Total, Input: row.Input, Output: row.Output,
				Reasoning: row.Reasoning, CacheRead: row.CacheRead, CacheWrite: row.CacheWrite, Sessions: row.Sessions}})
	}
	return result
}

func processingText(status application.Status) string {
	if status.Failed > 0 {
		return fmt.Sprintf("Processing: %s pending, %s failed; automatic retry scheduled", formatCount(status.Pending), formatCount(status.Failed))
	}
	if status.Pending > 0 {
		return "Processing: " + formatCount(status.Pending) + " pending"
	}
	return "Processing complete"
}

func collectionText(progress collectorprogress.DetailSnapshot) (string, bool) {
	if len(progress.Collection.Attempts) == 0 {
		return "Collection idle", false
	}
	latest := progress.Collection.Attempts[len(progress.Collection.Attempts)-1]
	switch latest.Stage {
	case "failed", "interrupted":
		return "Collection failed; saved usage remains available", true
	case "accepted":
		return "Collection complete", false
	case "capturing":
		var checked, total int64
		known := true
		for _, capture := range progress.Captures[latest.AttemptID] {
			checked += capture.Checked
			total += capture.Total
			known = known && capture.TotalKnown
		}
		if known && total > 0 {
			return fmt.Sprintf("Collecting: %s of %s sources checked", formatCount(checked), formatCount(total)), false
		}
		return "Discovering local sources…", false
	case "submitting":
		return "Saving collected usage…", false
	default:
		return "Collection waiting…", false
	}
}
