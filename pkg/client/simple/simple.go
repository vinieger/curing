package simple

import (
	"encoding/gob"
	"fmt"
	"github.com/amitschendel/curing/pkg/client"
	"github.com/amitschendel/curing/pkg/common"
	"net"
)

type SimpleClient struct {
	encoder *gob.Encoder
	decoder *gob.Decoder
}

func NewSimpleClient(conn net.Conn) *SimpleClient {
	return &SimpleClient{
		encoder: gob.NewEncoder(conn),
		decoder: gob.NewDecoder(conn),
	}
}

var _ client.Client = (*SimpleClient)(nil)

func (c SimpleClient) GetCommands() ([]common.Command, error) {
	// send request
	req := &common.Request{
		Type: common.GetCommands,
	}
	if err := c.encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	// get commands
	var cmds []common.Command
	if err := c.decoder.Decode(&cmds); err != nil {
		return nil, fmt.Errorf("failed to decode commands: %w", err)
	}
	return cmds, nil
}

func (c SimpleClient) SendResults(results []common.Result) error {
	req := &common.Request{
		Type:    common.SendResults,
		Results: results,
	}
	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	return nil
}
