package main

import (
	"fmt"
	"log"
)

func _main() error {
	fmt.Println("lol TODO make a client")

	// TODO do a ping against server

	return nil
}

func main() {
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
