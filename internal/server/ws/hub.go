package ws

import (
	"encoding/json"
	"sync"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type Hub struct {
	clients map[*Client]bool
	mu      sync.RWMutex
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]bool)}
}

func (h *Hub) Broadcast(msg interface{}) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	client := &Client{conn: conn, send: make(chan []byte, 64), hub: h}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	go client.writePump()
	client.readPump()
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.CloseNow()
	}()
	ctx := c.conn.CloseRead(nil)
	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.CloseNow()
	ctx := c.conn.CloseRead(nil)
	for msg := range c.send {
		err := c.conn.Write(ctx, websocket.MessageText, msg)
		if err != nil {
			return
		}
	}
}