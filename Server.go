package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	ConnPort = ":3333"
	ConnType = "tcp"

	MaxClients = 10

	CmdPrefix = "/"
	CmdCreate = CmdPrefix + "create"
	CmdList   = CmdPrefix + "list"
	CmdJoin   = CmdPrefix + "join"
	CmdLeave  = CmdPrefix + "leave"
	CmdHelp   = CmdPrefix + "help"
	CmdName   = CmdPrefix + "name"
	CmdQuit   = CmdPrefix + "quit"

	ClientName = "Anonymous"
	ServerName = "Server"

	ErrorPrefix = "Error: "
	ErrorSend   = ErrorPrefix + "You cannot send messages in the lobby.\n"
	ErrorCreate = ErrorPrefix + "A chat room with that name already exists.\n"
	ErrorJoin   = ErrorPrefix + "A chat room with that name does not exist.\n"
	ErrorLeave  = ErrorPrefix + "You cannot leave the lobby.\n"

	NoticePrefix         = "Notice: "
	NoticeRoomJoin       = NoticePrefix + "\"%s\" joined the chat room.\n"
	NoticeRoomLeave      = NoticePrefix + "\"%s\" left the chat room.\n"
	NoticeRoomName       = NoticePrefix + "\"%s\" changed their name to \"%s\".\n"
	NoticeRoomDelete     = NoticePrefix + "Chat room is inactive and being deleted.\n"
	NoticePersonalCreate = NoticePrefix + "Created chat room \"%s\".\n"
	NoticePersonalName   = NoticePrefix + "Changed name to \"\".\n"

	MsgConnect = "Welcome to the server! Type \"/help\" to get a list of commands.\n"
	MsgFull    = "Server is full. Please try reconnecting later."

	ExpiryTime time.Duration = 7 * 24 * time.Hour
)

/* A Lobby receives messages on its channels,
and keeps track of the currently connected clients,
and currently created chat rooms */
type Lobby struct {
	clients   []*Client
	chatRooms map[string]*ChatRoom
	incoming  chan *Message
	join      chan *Client
	leave     chan *Client
	delete    chan *ChatRoom
}

/* Creates a lobby which beings listening over its channels */
func NewLobby() *Lobby {
	lobby := &Lobby{
		clients:   make([]*Client, 0),
		chatRooms: make(map[string]*ChatRoom),
		incoming:  make(chan *Message),
		join:      make(chan *Client),
		leave:     make(chan *Client),
		delete:    make(chan *ChatRoom),
	}

	lobby.Listen()
	return lobby
}

/* Starts a new thread which listens over the lobby's various channels */
func (lobby *Lobby) Listen() {
	go func() {
		for {
			select {
			case message := <-lobby.incoming:
				lobby.Parse(message)
			case client := <-lobby.join:
				lobby.Join(client)
			case client := <-lobby.leave:
				lobby.Leave(client)
			case chatRoom := <-lobby.delete:
				lobby.DeleteChatRoom(chatRoom)
			}
		}
	}()
}

/* Handles clients connecting to the lobby */
func (lobby *Lobby) Join(client *Client) {
	if len(lobby.clients) >= MaxClients {
		client.Quit()
		return
	}

	lobby.clients = append(lobby.clients, client)
	client.outgoing <- MsgConnect

	go func() {
		for message := range client.incoming {
			lobby.incoming <- message
		}

		lobby.leave <- client
	}()
}

/* Handles clients disconnecting from the lobby */
func (lobby *Lobby) Leave(client *Client) {
	if client.chatRoom != nil {
		client.chatRoom.Leave(client)
	}

	for i, otherClient := range lobby.clients {
		if client == otherClient {
			lobby.clients = append(lobby.clients[:i], lobby.clients[i+1:]...)
			break
		}
	}

	close(client.outgoing)
	log.Println("Closed client's outgoing channel")
}

/* Checks if the a channel has expired. If it has, the chat room is deleted
Otherwise, a signal is sent to the delete channel at its new expiry time */
func (lobby *Lobby) DeleteChatRoom(chatRoom *ChatRoom) {
	if chatRoom.expiry.After(time.Now()) {
		go func() {
			time.Sleep(chatRoom.expiry.Sub(time.Now()))
			lobby.delete <- chatRoom
		}()

		log.Println("Attempted to delete chat room")
	} else {
		chatRoom.Delete()
		delete(lobby.chatRooms, chatRoom.name)
		log.Println("Deleted chat room")
	}
}

/* Handles messages sent to the lobby
If the message contains a command, the command is executed by the lobby
Otherwise, the message is sent to the sender's current chatroom */
func (lobby *Lobby) Parse(message *Message) {
	switch {
	default:
		lobby.SendMessage(message)
	case strings.HasPrefix(message.text, CmdCreate):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CmdCreate+" "), "\n")
		lobby.CreateChatRoom(message.client, name)
	case strings.HasPrefix(message.text, CmdList):
		lobby.ListChatRooms(message.client)
	case strings.HasPrefix(message.text, CmdJoin):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CmdJoin+" "), "\n")
		lobby.JoinChatRoom(message.client, name)
	case strings.HasPrefix(message.text, CmdLeave):
		lobby.LeaveChatRoom(message.client)
	case strings.HasPrefix(message.text, CmdName):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CmdName+" "), "\n")
		lobby.ChangeName(message.client, name)
	case strings.HasPrefix(message.text, CmdHelp):
		lobby.Help(message.client)
	case strings.HasPrefix(message.text, CmdQuit):
		message.client.Quit()
	}
}

/* Attempts to send the given message to the client's current chat room
If they are not in a chat room, an error message is sent to the client */
func (lobby *Lobby) SendMessage(message *Message) {
	if message.client.chatRoom == nil {
		message.client.outgoing <- ErrorSend
		log.Println("Client tried to send message in lobby")
		return
	}

	message.client.chatRoom.Broadcast(message.String())
	log.Println("Client sent message")
}

/* Attempts to create a chat room with the given name,
provided that one does not already exist */
func (lobby *Lobby) CreateChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] != nil {
		client.outgoing <- ErrorCreate
		log.Println("Client tried to create a chatroom with a name already in use")
		return
	}

	chatRoom := NewChatRoom(name)
	lobby.chatRooms[name] = chatRoom

	go func() {
		time.Sleep(ExpiryTime)
		lobby.delete <- chatRoom
	}()

	client.outgoing <- fmt.Sprintf(NoticePersonalCreate, chatRoom.name)
	log.Println("Client created chatroom")
}

/* Attempts to add the client to the chat room with the given name,
provided that the chat room exists */
func (lobby *Lobby) JoinChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] == nil {
		client.outgoing <- ErrorJoin
		log.Println("Client tried to join a chat room that does not exist")
		return
	}

	if client.chatRoom != nil {
		lobby.LeaveChatRoom(client)
	}

	lobby.chatRooms[name].Join(client)
	log.Println("Client joined chatroom")
}

/* Removes the given client from their current chatroom */
func (lobby *Lobby) LeaveChatRoom(client *Client) {
	if client.chatRoom == nil {
		client.outgoing <- ErrorLeave
		log.Println("Client tried to leave the lobby")
		return
	}

	client.chatRoom.Leave(client)
	log.Println("Client left the chatroom")
}

/* Changes the client's name to the given name */
func (lobby *Lobby) ChangeName(client *Client, name string) {
	if client.chatRoom == nil {
		client.outgoing <- fmt.Sprintf(NoticePersonalName, name)
	} else {
		client.chatRoom.Broadcast(fmt.Sprintf(NoticeRoomName, client.name, name))
	}

	client.name = name
	log.Println("Client changed their name")
}

/* Sends to the client the list of chat rooms currently open */
func (lobby *Lobby) ListChatRooms(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Chat Rooms:\n"
	for name := range lobby.chatRooms {
		client.outgoing <- fmt.Sprintf("%s\n", name)
	}

	client.outgoing <- "\n"
	log.Println("Client listed chatrooms")
}

/* Sends to the client the list of possible commands to the client */
func (lobby *Lobby) Help(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Commands:\n"
	client.outgoing <- "/help - lists all commands\n"
	client.outgoing <- "/list - lists all chatrooms\n"
	client.outgoing <- "/create foo - creates a chatroom named foo\n"
	client.outgoing <- "/join foo - joins a chatroom named foo\n"
	client.outgoing <- "/leave - leaves the current chatroom\n"
	client.outgoing <- "/name foo - changes your name to foo\n"
	client.outgoing <- "/quit - quits the program\n"
	client.outgoing <- "\n"
	log.Println("Client requested help")
}

/* A chatroom contains the chat's name,
a list of the currently connected clients,
a history of the messages broadcast to the users in the channel,
 and the current time at which the chatroom will expire */
type ChatRoom struct {
	name     string
	clients  []*Client
	messages []string
	expiry   time.Time
}

/* Creates an empty chatroom with the given name,
and sets its expiry time to the current time + EXPIRY_TIME */
func NewChatRoom(name string) *ChatRoom {
	return &ChatRoom{
		name:     name,
		clients:  make([]*Client, 0),
		messages: make([]string, 0),
		expiry:   time.Now().Add(ExpiryTime),
	}
}

/* Adds the given client to the chatroom,
and sends them all messages that have been sent since the
creation of the chatroom */
func (chatRoom *ChatRoom) Join(client *Client) {
	client.chatRoom = chatRoom

	for _, message := range chatRoom.messages {
		client.outgoing <- message
	}

	chatRoom.clients = append(chatRoom.clients, client)
	chatRoom.Broadcast(fmt.Sprintf(NoticeRoomJoin, client.name))
}

/* Removes the given client from the chatroom */
func (chatRoom *ChatRoom) Leave(client *Client) {
	chatRoom.Broadcast(fmt.Sprintf(NoticeRoomLeave, client.name))

	for i, otherClient := range chatRoom.clients {
		if client == otherClient {
			chatRoom.clients = append(chatRoom.clients[:i], chatRoom.clients[i+1:]...)
			break
		}
	}

	client.chatRoom = nil
}

/* Sends the given message to all clients currently in the chatroom */
func (chatRoom *ChatRoom) Broadcast(message string) {
	chatRoom.expiry = time.Now().Add(ExpiryTime)
	chatRoom.messages = append(chatRoom.messages, message)

	for _, client := range chatRoom.clients {
		client.outgoing <- message
	}
}

/* Notifies the clients within the chatroom that it is being deleted,
and kicks them back into the lobby */
func (chatRoom *ChatRoom) Delete() {
	chatRoom.Broadcast(NoticeRoomDelete)

	for _, client := range chatRoom.clients {
		client.chatRoom = nil
	}
}

/* A client abstracts away the idea of a connection
into incoming and outgoing channels,
and stores some information about the client's state,
including their current name and chat room */
type Client struct {
	name     string
	chatRoom *ChatRoom
	incoming chan *Message
	outgoing chan string
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
}

/* Returns a new client from the given connection,
and starts a reader and writer which receive
and send information from the socket */
func NewClient(conn net.Conn) *Client {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	client := &Client{
		name:     ClientName,
		chatRoom: nil,
		incoming: make(chan *Message),
		outgoing: make(chan string),
		conn:     conn,
		reader:   reader,
		writer:   writer,
	}

	client.Listen()
	return client
}

/* Starts two threads which read from the client's outgoing channel
and write to the client's socket connection,
and read from the client's socket
and write to the client's incoming channel */
func (client *Client) Listen() {
	go client.Read()
	go client.Write()
}

/* Reads in strings from the client's socket,
formats them into messages, and puts them into
the client's incoming channel */
func (client *Client) Read() {
	for {
		str, err := client.reader.ReadString('\n')
		if err != nil {
			log.Println(err)
			break
		}

		message := NewMessage(time.Now(), client, strings.TrimSuffix(str, "\n"))
		client.incoming <- message
	}

	close(client.incoming)
	log.Println("Closed client's incoming channel read thread")
}

/* Reads in messages from the client's outgoing channel,
and writes them to the client's socket */
func (client *Client) Write() {
	for str := range client.outgoing {
		_, err := client.writer.WriteString(str)
		if err != nil {
			log.Println(err)
			break
		}

		err = client.writer.Flush()
		if err != nil {
			log.Println(err)
			break
		}
	}

	log.Println("Closed client's write thread")
}

/* Closes the client's connection.
Socket closing is by error checking,
so this takes advantage of that to simplify the code
and make sure all the threads are cleaned up */
func (client *Client) Quit() {
	client.conn.Close()
}

/* A Message contains information about the sender,
the time at which the message was sent,
and the text of the message.
This gives a convenient way of passing the necessary
information about a message from the client to the lobby */
type Message struct {
	time   time.Time
	client *Client
	text   string
}

/* Creates a new message with the given time, client and text */
func NewMessage(time time.Time, client *Client, text string) *Message {
	return &Message{
		time:   time,
		client: client,
		text:   text,
	}
}

/* Returns a string representation of the message */
func (message *Message) String() string {
	return fmt.Sprintf("%s - %s: %s\n", message.time.Format(time.Kitchen), message.client.name, message.text)
}

/* Creates a lobby, listens for client connections, and connects them to the lobby */
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	lobby := NewLobby()

	listener, err := net.Listen(ConnType, ConnPort)

	if err != nil {
		log.Println("Error: ", err)
		os.Exit(1)
	}

	defer listener.Close()
	log.Println("Listening on " + ConnPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error: ", err)
			continue
		}

		lobby.Join(NewClient(conn))
	}
}
