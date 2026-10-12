package dom

import (
	"strings"
	"unicode"
)

// MinTermLength is the shortest word the trigram index can find.
const MinTermLength = 3

// SearchTerms are the unique words of a text to search by, in any language: lowercase, at least MinTermLength
// letters, long ones cut by a few letters so that other forms of the word still contain them (питомцы/питомец,
// Rechnung/Rechnungen). There are no stop words: the storage drops the terms most of the user's records contain.
func SearchTerms(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	seen := map[string]bool{}
	var terms []string
	for _, word := range words {
		runes := []rune(word)
		if len(runes) < MinTermLength {
			continue
		}
		term := string(runes[:len(runes)-endingCut(len(runes))])
		if !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	return terms
}

// endingCut is how many letters of a word's ending are left out: inflected languages change the ending,
// the trigram index finds the rest inside any form of the word.
func endingCut(length int) int {
	switch {
	case length >= 10:
		return 4
	case length >= 8:
		return 3
	case length >= 6:
		return 2
	}
	return 0
}
