package app

import (
	"embed"
	"strings"

	"dasyra/internal/dom"
)

//go:embed prompts/*.txt
var prompts embed.FS

func promptText(name string, vars ...string) string {
	text, err := prompts.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic(err)
	}
	return strings.NewReplacer(vars...).Replace(string(text))
}

// systemPrompt keeps the getting-to-know guidance with partial profiles too: saving a language preference
// does not finish onboarding. The stable guidance precedes the profile; a check-in adds its own rules.
func systemPrompt(u *dom.User, checkIn bool, collections []string, cookbook bool) string {
	name := u.DisplayName()
	prompt := promptText("dasyra", "{name}", name) + promptText("onboarding", "{name}", name)
	if u.HasProfile() {
		prompt += "\n\nProfile of " + name + ":\n" + strings.TrimSpace(u.Profile)
	}
	if len(collections) > 0 {
		prompt += "\n\nCollections of " + name + ": " + strings.Join(collections, ", ")
	}
	if cookbook {
		prompt += "\n\n" + promptText("cookbook", "{name}", name)
	}
	if !checkIn {
		return prompt
	}

	// Only users without a profile get here with their last message unanswered, see CheckIns.needsNudge
	nudge := ""
	if !u.HasProfile() {
		nudge = "\nFor this one introductory nudge, an unanswered greeting is an exception: send one short, friendly, no-pressure nudge to " + name + " that is easy to answer, without repeating the previous question."
	}
	return prompt + promptText("checkin", "{name}", name, "{nudge}", nudge)
}

func reflectionPrompt(u *dom.User, today, journalName string) string {
	return promptText("reflection", "{name}", u.DisplayName(), "{journal}", dom.CategoryJournal, "{journalName}", journalName, "{today}", today)
}
