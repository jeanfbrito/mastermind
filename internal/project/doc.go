// Package project detects and normalizes the current project name.
//
// Detection priority: git remote origin repo name → git root basename →
// cwd basename. Normalized to lowercase, trimmed. This is the canonical
// "what project am I in?" answer used by session-start injection,
// session-close extraction, and project-scoped queries.
//
// The algorithm is adapted from engram's internal/project/detect.go
// (github.com/Gentleman-Programming/engram), re-implemented in a form
// compatible with mastermind's dependency profile.
package project
