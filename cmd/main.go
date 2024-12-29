package main

import "github.com/amitschendel/curing/pkg/executer"

func main() {
	_, err := executer.NewExecuter()
	if err != nil {
		panic(err)
	}
}
