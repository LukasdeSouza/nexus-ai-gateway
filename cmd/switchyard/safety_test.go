package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePolicy(t *testing.T) {
	p, err := ParsePolicy("explain")
	assert.NoError(t, err)
	assert.Equal(t, PolicyExplain, p)

	p, err = ParsePolicy("plan")
	assert.NoError(t, err)
	assert.Equal(t, PolicyPlan, p)

	p, err = ParsePolicy("approve")
	assert.NoError(t, err)
	assert.Equal(t, PolicyApprove, p)

	p, err = ParsePolicy("safe-auto")
	assert.NoError(t, err)
	assert.Equal(t, PolicySafeAuto, p)

	p, err = ParsePolicy("autopilot")
	assert.NoError(t, err)
	assert.Equal(t, PolicyAutopilot, p)

	_, err = ParsePolicy("nonexistent")
	assert.Error(t, err)
}

func TestIsProtectedFile(t *testing.T) {
	cases := []struct {
		path      string
		protected bool
	}{
		{".env", true},
		{".env.production", true},
		{".env.local", true},
		{"server.key", true},
		{"cert.pem", true},
		{"go.sum", true},
		{"package-lock.json", true},
		{".git/config", true},
		{"internal/routing/engine.go", false},
		{"cmd/switchyard/main.go", false},
		{"README.md", false},
	}

	for _, tc := range cases {
		isP, _ := IsProtectedFile(tc.path)
		assert.Equal(t, tc.protected, isP, "path: %s", tc.path)
	}
}

func TestPrintExecutionPlanManifest(t *testing.T) {
	edits := []ProposedEdit{
		{FilePath: "index.html", Action: "write", NewText: "<h1>Hello</h1>"},
		{FilePath: ".env", Action: "write", NewText: "SECRET=123"},
		{FilePath: "main.go", Action: "edit", OldText: "foo", NewText: "bar"},
	}

	// Should not panic and properly format
	assert.NotPanics(t, func() {
		PrintExecutionPlanManifest(edits, PolicyApprove)
		PrintExecutionPlanManifest(edits, PolicySafeAuto)
		PrintExecutionPlanManifest(edits, PolicyExplain)
	})
}

func TestEstimateCost(t *testing.T) {
	cost := estimateCost("explore", 1000, 500)
	assert.Greater(t, cost, 0.0)

	costBuild := estimateCost("build", 1000, 500)
	assert.Greater(t, costBuild, cost)
}