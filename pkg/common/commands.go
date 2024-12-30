package common

type CommandRequest struct {
	AgentID string
}

type Command interface {
	ID() string
}
