package tui

import (
	"sort"
	"strings"
	"unicode"
)

// How well a command answers one token of the query. A command has to score
// against every token or it is not listed at all, and the tiers are what put
// "Clear filters" above "Group issues by…" for "cle" — one starts with the
// token, the other carries a keyword that happens to contain it.
const (
	scoreTitlePrefix     = 100
	scoreTitleWordPrefix = 80
	scoreTitleContains   = 60
	scoreKeywordPrefix   = 40
	scoreKeywordContains = 20
	// The fuzzy tiers are only ever reached when nothing matched a run of the
	// query's own characters, so they never push a real match down the list.
	scoreTitleFuzzy   = 10
	scoreKeywordFuzzy = 5
)

// scoredCommand pairs a command with what it scored against the whole query.
type scoredCommand struct {
	command Command
	score   int
}

// rankCommands returns the commands matching every token of the query, best
// first, ties broken alphabetically so the order never moves under a redraw.
// An empty query ranks nothing and returns the commands as given.
func rankCommands(commands []Command, query string) []Command {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return commands
	}

	ranked := scoreCommands(commands, tokens, substringScore)
	if len(ranked) == 0 {
		// Nothing holds the query as a run of characters. Scatter them through
		// the titles and keywords instead, which is what turns "stng" into
		// Settings rather than an empty list.
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

// scoreCommands keeps the commands that answer every token under the given
// scorer, carrying the sum of what each token scored.
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

// substringScore ranks a command by where the token sits in its title, and
// failing that, in its keywords.
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

// fuzzyScore ranks a command by whether the token's characters appear in order
// in its title or one of its keywords, however far apart.
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

// hasWordPrefix reports whether the token starts a word of the text. A word
// begins at the start of the string or after anything that is not a letter or
// a digit, so "sub" and "issue" both start a word of "create sub-issue".
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

// isWordRune reports whether the rune is part of a word rather than a break
// between two.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// isSubsequence reports whether every rune of the token appears in the target
// in order, with anything allowed between them.
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
