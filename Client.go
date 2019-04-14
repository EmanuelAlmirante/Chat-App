package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

const (
	ConnPort = ":3333"
	ConnType = "tcp"

	MsgDisconnect = "Disconnected.\n"
)

var waitGroup sync.WaitGroup

/* Read messages received and prints on the console */
func Read(conn net.Conn) {
	reader := bufio.NewReader(conn)

	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf(MsgDisconnect)
			waitGroup.Done()
			return
		}

		fmt.Print(str)
	}
}

/* Write message that is read from the console on
the chat to the other users */
func Write(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(conn)

	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		_, err = writer.WriteString(str)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		err = writer.Flush()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

/* Start a read and write thread that connects to the
server through the socket connetion */
func main() {
	waitGroup.Add(1)

	conn, err := net.Dial(ConnType, ConnPort)
	if err != nil {
		fmt.Println(err)
	}

	go Read(conn)
	go Write(conn)

	waitGroup.Wait()
}
