package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

func saveBenchReport(dir string, report benchReport) error {
	if err := writeBenchJSON(filepath.Join(dir, "results.json"), report); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt bench\n\nStarted: %s\n\nRevision: %s (modified: %t)\n\n", report.Started.Format("2006-01-02 15:04:05Z07:00"), report.Revision, report.Modified)
	fmt.Fprintf(&b, "Model: %s; effort: %s; provider: %s; dry run: %t.\n\n", defaultLabel(report.Options.Model), defaultLabel(report.Options.Effort), report.Options.Kind, report.Options.DryRun)
	fmt.Fprintf(&b, "Semantic search: %t (disabled during dry runs).\n\n", !report.Options.NoEmbeddings)
	b.WriteString("Human review only. Review notes are not sent to the model. External tools use fixtures. Each scenario starts in a fresh database.\n\n")
	if len(report.Options.Roles) > 0 {
		data, _ := json.MarshalIndent(report.Options.Roles, "", "  ")
		b.WriteString("Role settings:\n\n" + benchFence(string(data)) + "\n")
	}
	if report.Options.DryRun {
		b.WriteString("This is a prompt inspection: NO_MESSAGE is a stub response, not a model result.\n\n")
	}
	for _, r := range report.Results {
		b.WriteString(benchCaseText(r))
	}
	return writeBenchFile(filepath.Join(dir, "report.md"), []byte(b.String()))
}

func defaultLabel(s string) string {
	if s == "" {
		return "role defaults"
	}
	return s
}

func benchCaseText(r benchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## %s — %s\n\n%s / %s / repeat %d / %.1fs\n\n", r.Case.ID, r.Case.Title, r.Case.Group, r.Case.Mode, r.Repeat, r.Seconds)
	fmt.Fprintf(&b, "Scenario time: %s (%s).\n\n", r.Case.At, r.Case.User.Timezone)
	if r.Case.Source != "" {
		fmt.Fprintf(&b, "Source: %s\n\n", r.Case.Source)
	}
	b.WriteString("Review:\n\n")
	for _, note := range r.Case.Review {
		fmt.Fprintf(&b, "- %s\n", note)
	}
	b.WriteString("\nContext:\n\n")
	var context strings.Builder
	fmt.Fprintf(&context, "Profile: %s\n", r.Case.User.Profile)
	for _, m := range r.Case.History {
		fmt.Fprintf(&context, "%s: %s\n", m.Role, m.Text)
	}
	if r.Case.Message != "" {
		fmt.Fprintf(&context, "user NOW: %s\n", r.Case.Message)
	}
	b.WriteString(benchFence(context.String()))
	b.WriteString("\nDelivered:\n\n")
	if len(r.Sent) == 0 && len(r.Reactions) == 0 {
		b.WriteString("(Nothing sent.)\n")
	}
	for _, text := range r.Sent {
		b.WriteString(benchFence(text))
	}
	for _, reaction := range r.Reactions {
		fmt.Fprintf(&b, "Reaction: %s\n", reaction)
	}
	if len(r.Steps) > 0 {
		last := r.Steps[len(r.Steps)-1]
		fmt.Fprintf(&b, "\nModel: %s / %s; calls: %d. Final raw text:\n\n", last.Request.Model.ID, last.Request.Model.Effort, len(r.Steps))
		b.WriteString(benchFence(last.Reply.Text))
	}
	b.WriteString("\nTools:\n\n")
	if len(r.Calls) == 0 {
		b.WriteString("(None.)\n")
	}
	for _, call := range r.Calls {
		b.WriteString(benchFence(call.Tool + " " + call.Args + "\n" + call.Result))
	}
	b.WriteString("\nState changes:\n\n")
	changes := false
	for _, table := range []string{"profile", "memories", "events", "todos", "notes"} {
		before, after := rowsByID(r.Before[table]), rowsByID(r.After[table])
		for _, row := range r.Before[table] {
			id := fmt.Sprint(row["id"])
			next, exists := after[id]
			if !exists || !reflect.DeepEqual(row, next) {
				changes = true
				fmt.Fprintf(&b, "%s #%s:\n\n", table, id)
				b.WriteString(benchFence("Before: " + rowJSON(row) + "\nAfter: " + rowJSON(next)))
			}
		}
		for _, row := range r.After[table] {
			id := fmt.Sprint(row["id"])
			if _, exists := before[id]; !exists {
				changes = true
				fmt.Fprintf(&b, "%s #%s added:\n\n", table, id)
				b.WriteString(benchFence(rowJSON(row)))
			}
		}
	}
	if !changes {
		b.WriteString("(None.)\n")
	}
	// The initial fixture records matter even when the model leaves them untouched.
	b.WriteString("\n<details><summary>Initial stored state</summary>\n\n")
	data, _ := json.MarshalIndent(r.Before, "", "  ")
	b.WriteString(benchFence(string(data)))
	b.WriteString("\n</details>\n")
	if len(r.Usage) > 0 {
		data, _ := json.MarshalIndent(r.Usage, "", "  ")
		b.WriteString("\nUsage:\n\n" + benchFence(string(data)))
	}
	if r.Error != "" {
		b.WriteString("\nExecution error:\n\n" + benchFence(r.Error))
	}
	return b.String()
}

func rowsByID(rows []benchRow) map[string]benchRow {
	out := map[string]benchRow{}
	for _, row := range rows {
		out[fmt.Sprint(row["id"])] = row
	}
	return out
}

func rowJSON(row benchRow) string {
	if row == nil {
		return "(absent)"
	}
	data, _ := json.Marshal(row)
	return string(data)
}

// A model answer can contain code fences; choose a longer fence without changing its text.
func benchFence(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "text\n" + text + "\n" + fence + "\n"
}
