package common

import (
	"encoding/gob"
	"fmt"
)

func init() {
	gob.Register(Symlink{})
}

type Symlink struct {
	Id      string
	OldPath string // The path to the target (existing file)
	NewPath string // The path where the symlink will be created
}

var _ Command = (*Symlink)(nil)

func (s Symlink) ID() string {
	return s.Id
}

func (s Symlink) String() string {
	return fmt.Sprintf("%s - create symlink: %s -> %s", s.Id, s.NewPath, s.OldPath)
}
