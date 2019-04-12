package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

const (
	CONN_PORT = ":1000"
	CONN_TYPE = "tcp"

	MSG_DISCONNECT = "Disconnected.\n"
)

var waitGroup sync.WaitGroup

// Reads messages received and prints on the console
func Read(conn net.Conn) {

}


// Reads message from the console and sends to other users
func Write(conn net.Conn) {

}


// Start a read and write thread that connects to the server through the socket connetion
func main() {
	waitGroup.Add(1)

	conn, err := net.Dial(CONN_TYPE, CONN_PORT)
	if err != nil {
		fmt.Println(err)
	}

	go Read(conn)
	go Write(conn)

	waitGroup.Wait()
}