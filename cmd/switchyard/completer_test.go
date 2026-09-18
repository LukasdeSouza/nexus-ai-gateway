package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSwitchyardCompleter(t *testing.T) {
	c := NewSwitchyardCompleter()

	// 1. Slash commands
	candidates, matchLen := c.Do([]rune("/"), 1)
	assert.Equal(t, 1, matchLen)
	assert.NotEmpty(t, candidates)

	// 2. /pol -> /policy
	candidates, matchLen = c.Do([]rune("/pol"), 4)
	assert.Equal(t, 4, matchLen)
	var foundPolicy bool
	for _, cand := range candidates {
		if string(cand) == "icy" {
			foundPolicy = true
		}
	}
	assert.True(t, foundPolicy)

	// 3. /policy safe -> safe-auto
	candidates, matchLen = c.Do([]rune("/policy safe"), 12)
	assert.Equal(t, 4, matchLen) // "safe" is 4 runes
	assert.NotEmpty(t, candidates)
	assert.Equal(t, "-auto", string(candidates[0]))

	// 4. /preset bui -> build
	candidates, matchLen = c.Do([]rune("/preset bui"), 11)
	assert.Equal(t, 3, matchLen) // "bui" is 3 runes
	assert.NotEmpty(t, candidates)
	assert.Equal(t, "ld", string(candidates[0]))

	// 5. File autocomplete
	candidates, matchLen = c.Do([]rune("Check @safe"), 11)
	assert.Equal(t, 4, matchLen) // "safe" is 4 runes
	var foundSafety bool
	for _, cand := range candidates {
		if string(cand) == "ty.go" {
			foundSafety = true
		}
	}
	assert.True(t, foundSafety)
}