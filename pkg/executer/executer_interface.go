package executer

type IExecuter interface {
	SetCommandsChannel(commands chan string) // TODO: specify the type of the channel.
	GetOutputChannel() chan string           // TODO: specify the type of the channel.
}
