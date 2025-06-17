package core

import "time"

// Event represents a message exchanged between agents.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

// BlackboardUpdate can be emitted when blackboard entries change.
type BlackboardUpdate struct {
	Key   string
	Value interface{}
}
