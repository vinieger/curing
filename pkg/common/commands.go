package common

import "encoding/gob"

func init() {
	gob.Register(CommandList{})
	gob.Register([]Command{})
}

type CommandRequest struct {
	AgentID string
}

type CommandList struct {
	Commands []Command
}

type Command interface {
	ID() string
}
