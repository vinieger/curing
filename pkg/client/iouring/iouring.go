package iouring

import (
	"github.com/amitschendel/curing/pkg/client"
	"github.com/amitschendel/curing/pkg/common"
)

type IOUringClient struct {
}

var _ client.Client = (*IOUringClient)(nil)

func (c IOUringClient) GetCommands() ([]common.Command, error) {
	//TODO implement me
	panic("implement me")
}

func (c IOUringClient) SendResults(results []common.Result) error {
	//TODO implement me
	panic("implement me")
}
