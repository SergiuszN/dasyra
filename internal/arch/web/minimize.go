package web

import (
	"html"
	"regexp"
	"strings"
)

const minDivText = 100

var (
	noise = []*regexp.Regexp{
		block("script"), block("style"), block("head"), block("header"), block("footer"), block("nav"), block("noscript"),
		regexp.MustCompile(`(?s)<!--.*?-->`),
	}
	divPattern  = regexp.MustCompile(`(?is)<div\b[^>]*>(.*?)</div>`)
	betweenTags = regexp.MustCompile(`>\s+<`)
	tagPattern  = regexp.MustCompile(`<[^>]*>`)
)

func block(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>.*?</` + tag + `>`)
}

// minimize turns a page into its main text: drops scripts, chrome and short divs (menus, buttons, cookie bars).
func minimize(page string) string {
	text := html.UnescapeString(strings.ToValidUTF8(page, "?"))
	for _, re := range noise {
		text = re.ReplaceAllString(text, "")
	}
	// Nested divs may become short after inner ones are removed, so repeat until nothing changes
	for previous := -1; len(text) != previous; {
		previous = len(text)
		text = removeShortDivs(text)
	}
	text = betweenTags.ReplaceAllString(text, "><")
	text = spaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(stripTags(text))
}

func removeShortDivs(text string) string {
	return divPattern.ReplaceAllStringFunc(text, func(div string) string {
		inner := divPattern.FindStringSubmatch(div)[1]
		if len(strings.TrimSpace(stripTags(inner))) < minDivText {
			return ""
		}
		return div
	})
}

func stripTags(s string) string {
	return tagPattern.ReplaceAllString(s, "")
}
