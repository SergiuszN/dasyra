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

// systemPrompt is Dasyra's persona with the user's profile, or the onboarding rules while there is none;
// a check-in adds the proactive rules.
func systemPrompt(u *dom.User, checkIn bool, collections []string) string {
	name := u.DisplayName()
	prompt := promptText("dasyra", "{name}", name)
	if u.HasProfile() {
		prompt += "\n\nProfile of " + name + ":\n" + strings.TrimSpace(u.Profile)
	} else {
		prompt += promptText("onboarding", "{name}", name)
	}
	if len(collections) > 0 {
		prompt += "\n\nCollections of " + name + ": " + strings.Join(collections, ", ")
	}
	if !checkIn {
		return prompt
	}

	// Only users without a profile get here with their last message unanswered, see CheckIns.needsNudge
	nudge := ""
	if !u.HasProfile() {
		nudge = "\nIf " + name + " did not reply to your last message, send one short, friendly, no-pressure nudge that is easy to answer, without repeating your previous question."
	}
	return prompt + promptText("checkin", "{name}", name, "{nudge}", nudge)
}

func reflectionPrompt(u *dom.User, today, journalName string) string {
	return promptText("reflection", "{name}", u.DisplayName(), "{journal}", dom.CategoryJournal, "{journalName}", journalName, "{today}", today)
}
