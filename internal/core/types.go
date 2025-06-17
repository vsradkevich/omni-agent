package core

// Status represents execution result of actions or nodes.
type Status int

const (
	StatusSuccess Status = iota
	StatusFailure
	StatusRunning
)
