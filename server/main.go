package main

import (
	"fmt"
	"net"
	"os"
)

func _main() (err error) {
	var listener net.Listener
	listener, err = net.Listen("tcp", "127.0.0.1:6666")
	if err != nil {
		return
	}

	defer listener.Close()

	for {
		var conn net.Conn
		conn, err = listener.Accept()
		if err != nil {
			// TODO log and continue
			break
		}

		go handleConnection(conn)
	}

	return
}

func handleConnection(conn net.Conn) {
	// TODO create a user session
	fmt.Println("HI")

	// TODO learn how to read from here -> protobuff
	for {
		var bs []byte
		// TODO how to block here?
		conn.Read(bs)
		fmt.Printf("DBG %#v\n", string(bs))
	}
}

func main() {
	fmt.Println("hi lol")
	err := _main()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
