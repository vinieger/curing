//go:build linux

package main

import (
	"fmt"
)

func main() {
	fmt.Println("Hello, world!")
	// cfg, err := config.LoadConfig("config.json")
	// if err != nil {
	// 	panic(err)
	// }

	// ctx := context.Background()

	// e, err := executer.NewExecuter(cfg, ctx)
	// if err != nil {
	// 	panic(err)
	// }

	// // Set the commands channel
	// e.SetCommandsChannel(make(chan string)) // TODO: put the real channel here.

	// // Start the executer (this is a blocking call)
	// e.Run()
}
