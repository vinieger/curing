package mock

import "github.com/amitschendel/curing/pkg/common"

type MockClient interface {
	GetCommands() ([]common.Command, error)
	SendResults([]common.Result) error
}
