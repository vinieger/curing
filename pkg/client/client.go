package client

import "github.com/amitschendel/curing/pkg/common"

type Client interface {
	GetCommands() ([]common.Command, error)
	SendResults([]common.Result) error
}
