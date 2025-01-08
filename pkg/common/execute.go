package common

import (
	"encoding/gob"
	"fmt"
)

func init() {
	gob.Register(Execute{})
}

type Execute struct {
	Id      string
	Command string
}

var _ Command = (*Execute)(nil)

func (e Execute) ID() string {
	return e.Id
}

func (e Execute) String() string {
	return fmt.Sprintf("%s - execute command: %s}", e.Id, e.Command)
}
