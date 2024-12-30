package simple

import (
	"fmt"
	"github.com/amitschendel/curing/pkg/common"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleClient_GetCommands(t *testing.T) {
	conn, err := net.Dial("tcp", "localhost:8080")
	require.NoError(t, err)
	s := NewSimpleClient(conn)
	commands, err := s.GetCommands()
	assert.NoError(t, err)
	assert.NotNil(t, commands)
	for _, c := range commands {
		fmt.Println(c)
	}
}

func TestSimpleClient_SendResults(t *testing.T) {
	conn, err := net.Dial("tcp", "localhost:8080")
	require.NoError(t, err)
	s := NewSimpleClient(conn)
	results := []common.Result{
		{CommandID: "command1", ReturnCode: 0},
		{CommandID: "command2", ReturnCode: 1, Output: "permission denied"},
	}
	err = s.SendResults(results)
	assert.NoError(t, err)
}
