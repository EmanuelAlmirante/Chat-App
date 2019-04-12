package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

var x string

func server() {
	// Listen on a port
	port, err := net.Listen("tcp", ":1000")
	
	if (err != nil) {
		fmt.Println(err)
		return
	}

	for {
		// Accept a connection
		conn, err := port.Accept()

		if (err != nil) {
			fmt.Println(err)
			continue
		} else {
			go handleServerConnection(conn)
		}
	}
}

func handleServerConnection(conn net.Conn) {
	// Receive the message
	var msg string
	err := gob.NewDecoder(conn).Decode(&msg)

	if (err != nil) {
		fmt.Println(err)
	} else {
		fmt.Println(msg)
	}
}

func main() {
	go server()
}