package common

import (
	"encoding/gob"
	"fmt"
)

func init() {
	gob.Register(WriteFile{})
}

type WriteFile struct {
	Id      string
	Content string
	Path    string
}

var _ Command = (*WriteFile)(nil)

func (w WriteFile) ID() string {
	return w.Id
}

func (w WriteFile) String() string {
	return fmt.Sprintf("%s - create file: %s", w.Id, w.Path)
}
