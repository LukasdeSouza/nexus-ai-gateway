package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompleterSlashCommands(t *testing.T) {
	c := NewNexusCompleter()

	// 1. Typing "/"
	candidates, matchLen := c.Do([]rune("/"), 1)
	assert.Equal(t, 1, matchLen)
	assert.NotEmpty(t, candidates)

	// 2. Typing "/mo"
	candidates, matchLen = c.Do([]rune("/mo"), 3)
	assert.Equal(t, 3, matchLen)
	var words []string
	for _, cand := range candidates {
		words = append(words, "/mo"+string(cand))
	}
	assert.Contains(t, words, "/model")
	assert.Contains(t, words, "/models")

	// 3. Typing "/model "
	candidates, matchLen = c.Do([]rune("/model "), 7)
	assert.Equal(t, 0, matchLen)
	assert.NotEmpty(t, candidates)

	// 4. Typing "/model fast"
	candidates, matchLen = c.Do([]rune("/model fas"), 10)
	assert.Equal(t, 3, matchLen)
	assert.Equal(t, 1, len(candidates))
	assert.Equal(t, "t", string(candidates[0]))

	// 5. Typing "/auto "
	candidates, matchLen = c.Do([]rune("/auto "), 6)
	assert.Equal(t, 0, matchLen)
	assert.Equal(t, 2, len(candidates))
}

func TestCompleterAtFilePath(t *testing.T) {
	c := NewNexusCompleter()

	// 1. "@" alone in current working directory
	candidates, matchLen := c.Do([]rune("@"), 1)
	assert.Equal(t, 0, matchLen)
	assert.NotEmpty(t, candidates)

	// 2. In middle of sentence: "Please inspect @comp"
	line := []rune("Please inspect @comp")
	candidates, matchLen = c.Do(line, len(line))
	assert.Equal(t, 4, matchLen) // "comp" is 4 chars
	var foundCompleter bool
	for _, cand := range candidates {
		if string(cand) == "leter.go" {
			foundCompleter = true
		}
	}
	assert.True(t, foundCompleter, "expected 'leter.go' suffix for @comp")
}