package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

const (
	maxClients = 1000
)

var serverPort = flag.Int("p", 8973, "server port")

type Client struct {
	conn     net.Conn
	nick     string
	readChan chan string
}

type ChatState struct {
	listener net.Listener

	clients     map[net.Conn]*Client
	clientsLock sync.RWMutex
	numClients  int
}

var chatState = &ChatState{
	clients: make(map[net.Conn]*Client),
}

func initChat() {
	serverPort := fmt.Sprintf(":%d", *serverPort)

	var err error
	chatState.listener, err = net.Listen("tcp", serverPort)
	if err != nil {
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
}

// 客户端处理逻辑
func handleClient(client *Client) {
	welcomeMsg := "Welcome Simple Chat! Use /nick to change nick name. \n"
	client.conn.Write([]byte(welcomeMsg))

	buf := make([]byte, 512)
	for {
		n, err := client.conn.Read(buf)
		if err != nil {
			fmt.Printf("client left: %s\n", client.conn.RemoteAddr())
			closeClient(client)
			return
		}

		msg := string(buf[:n])
		msg = strings.TrimSpace(msg)
		if len(msg) > 0 && msg[0] == '/' {
			// 处理命令
			parts := strings.SplitN(msg, " ", 2)
			cmd := parts[0]
			if cmd == "/nick" && len(parts) > 1 {
				if len(parts[1]) > 32 {
					client.conn.Write([]byte("nick name too long\n"))
					continue
				}
				client.nick = parts[1]
			}
			continue
		}

		if len(msg) == 0 {
			continue
		}
		if buf[0] == 255 || strings.ToLower(msg) == "quit" {
			closeClient(client)
			return
		}

		fmt.Printf("%s: %s\n", client.nick, msg)

		// 将消息转发给其他客户端
		chatState.clientsLock.RLock()
		for _, cl := range chatState.clients {
			if cl != client {
				cl.readChan <- ">> " + client.nick + ": " + msg + "\n"
			}
		}
		chatState.clientsLock.RUnlock()
	}
}

// 向各客户端发送信息
func (client Client) startReceive() {
	for msg := range client.readChan {
		client.conn.Write([]byte(msg))
	}
}

func closeClient(client *Client) {
	chatState.clientsLock.Lock()
	close(client.readChan)
	client.conn.Close()
	delete(chatState.clients, client.conn)
	chatState.numClients--
	chatState.clientsLock.Unlock()
}

func main() {
	// 解析命令行内容
	flag.Parse()

	fmt.Println(*serverPort)
	// 初始化服务端端口
	initChat()

	for {
		conn, err := chatState.listener.Accept()
		if err != nil {
			fmt.Println("accept err:", err)
			os.Exit(1)
		}

		client := &Client{conn: conn}
		client.nick = fmt.Sprintf("user%d", conn.RemoteAddr().(*net.TCPAddr).Port)
		client.readChan = make(chan string, 5)

		// 加锁，让新进入的客户端进入
		chatState.clientsLock.Lock()
		if chatState.numClients >= maxClients {
			fmt.Printf("too many clients, reject %s\n", conn.RemoteAddr())
			conn.Close()
			chatState.clientsLock.Unlock()
			continue
		}
		chatState.clients[conn] = client
		chatState.numClients++
		chatState.clientsLock.Unlock()

		go handleClient(client)
		go client.startReceive()
		fmt.Printf("new Client: %s\n", conn.RemoteAddr())
	}
}