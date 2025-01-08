package common

type RequestType int

const (
	GetCommands RequestType = iota
	SendResults
)

var typeName = map[RequestType]string{
	GetCommands: "GetCommands",
	SendResults: "SendResults",
}

func (rt RequestType) String() string {
	return typeName[rt]
}

type Request struct {
	AgentID string
	Type    RequestType
	Results []Result
}

type Result struct {
	CommandID  string
	ReturnCode int
	Output     string
}
