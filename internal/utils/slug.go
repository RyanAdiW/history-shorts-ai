package utils

import (
	"regexp"
	"strings"
	"unicode"
)

func TopicSlug(topic string) string {
	return Slugify(topicSlugSource(topic))
}

func Slugify(input string) string {
	var b strings.Builder
	lastDash := false

	for _, r := range strings.ToLower(input) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || strings.ContainsRune("-_:/.,?!'\"()[]{}", r):
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	slug := strings.Trim(b.String(), "-")
	return regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")
}

func topicSlugSource(topic string) string {
	cleaned := strings.TrimSpace(topic)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^why\s+did\s+(.+?)\s+(?:die|fall|collapse|disappear|vanish|fail|win|lose|end|begin|start|happen)\b.*$`),
		regexp.MustCompile(`(?i)^what\s+(?:was|is)\s+(.+)$`),
		regexp.MustCompile(`(?i)^who\s+(?:was|is)\s+(.+)$`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(cleaned)
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return cleaned
}
