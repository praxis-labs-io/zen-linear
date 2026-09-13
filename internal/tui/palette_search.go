package tui

import (
	"sort"
	"strings"
	"unicode"
)

const (
	scoreTitlePrefix     = 100
	scoreTitleWordPrefix = 80
	scoreTitleContains   = 60
	scoreKeywordPrefix   = 40
	scoreKeywordContains = 20
	scoreTitleFuzzy      = 10
	scoreKeywordFuzzy    = 5
)

type scoredCommand struct {
	command Command
	score   int
}

func rankCommands(commands []Command, query string) []Command {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return commands
	}

	ranked := scoreCommands(commands, tokens, substringScore)
	if len(ranked) == 0 {
		ranked = scoreCommands(commands, tokens, fuzzyScore)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].command.Title < ranked[j].command.Title
	})

	matched := make([]Command, len(ranked))
	for i, candidate := range ranked {
		matched[i] = candidate.command
	}
	return matched
}

func scoreCommands(commands []Command, tokens []string, score func(Command, string) int) []scoredCommand {
	matched := make([]scoredCommand, 0, len(commands))
	for _, cmd := range commands {
		total := 0
		for _, token := range tokens {
			hit := score(cmd, token)
			if hit == 0 {
				total = 0
				break
			}
			total += hit
		}
		if total > 0 {
			matched = append(matched, scoredCommand{command: cmd, score: total})
		}
	}
	return matched
}

func substringScore(cmd Command, token string) int {
	title := strings.ToLower(cmd.Title)
	switch {
	case strings.HasPrefix(title, token):
		return scoreTitlePrefix
	case hasWordPrefix(title, token):
		return scoreTitleWordPrefix
	case strings.Contains(title, token):
		return scoreTitleContains
	}

	best := 0
	for _, keyword := range cmd.Keywords {
		keyword = strings.ToLower(keyword)
		if hasWordPrefix(keyword, token) {
			return scoreKeywordPrefix
		}
		if strings.Contains(keyword, token) {
			best = scoreKeywordContains
		}
	}
	return best
}

func fuzzyScore(cmd Command, token string) int {
	if isSubsequence(token, strings.ToLower(cmd.Title)) {
		return scoreTitleFuzzy
	}
	for _, keyword := range cmd.Keywords {
		if isSubsequence(token, strings.ToLower(keyword)) {
			return scoreKeywordFuzzy
		}
	}
	return 0
}

func hasWordPrefix(text, token string) bool {
	var previous rune
	for offset, r := range text {
		if (offset == 0 || !isWordRune(previous)) && strings.HasPrefix(text[offset:], token) {
			return true
		}
		previous = r
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isSubsequence(token, target string) bool {
	needle := []rune(token)
	if len(needle) == 0 {
		return true
	}
	at := 0
	for _, r := range target {
		if r != needle[at] {
			continue
		}
		at++
		if at == len(needle) {
			return true
		}
	}
	return false
}
