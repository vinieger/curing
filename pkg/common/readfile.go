package common

import (
	"encoding/gob"
	"fmt"
)

func init() {
	gob.Register(ReadFile{})
}

type ReadFile struct {
	Id   string
	Path string
}

var _ Command = (*ReadFile)(nil)

func (w ReadFile) ID() string {
	return w.Id
}

func (w ReadFile) String() string {
	return fmt.Sprintf("%s - read file: %s", w.Id, w.Path)
}
