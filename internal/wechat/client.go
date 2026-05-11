package wechat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const DefaultBaseURL = "https://ilinkai.weixin.qq.com"

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusExpired      Status = "expired"
)

type Client struct {
	baseURL       string
	botToken      string
	loginTime     time.Time
	status        Status
	mu            sync.RWMutex
	httpClient    *http.Client
	msgCh         chan Message
	getUpdatesBuf string
	lastContact   ContactInfo
	typingTickets map[string]string
	stopCh        chan struct{}
	done          chan struct{}
	stopOnce      sync.Once
	onContact     func(ContactInfo)
}

type ContactInfo struct {
	FromID       string
	ContextToken string
}

func NewClient(baseURL, botToken string, loginTime time.Time) *Client {
	return &Client{
		baseURL:       baseURL,
		botToken:      botToken,
		loginTime:     loginTime,
		status:        StatusDisconnected,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		msgCh:         make(chan Message, 100),
		typingTickets: make(map[string]string),
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
	}
}

func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Client) SetStatus(s Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.botToken
}

func (c *Client) SetContactCallback(fn func(ContactInfo)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onContact = fn
}

func (c *Client) SetToken(token, baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.botToken = token
	if baseURL != "" {
		c.baseURL = baseURL
	}
	c.loginTime = time.Now()
}

func (c *Client) LoginTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loginTime
}

func (c *Client) Messages() <-chan Message { return c.msgCh }

func (c *Client) SetLastContact(ci ContactInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastContact = ci
}

func (c *Client) LastContact() ContactInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastContact
}

func (c *Client) makeHeaders() map[string]string {
	h := map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
	}
	uin := fmt.Sprintf("%d", rand.Uint32())
	h["X-WECHAT-UIN"] = base64.StdEncoding.EncodeToString([]byte(uin))
	if c.Token() != "" {
		h["Authorization"] = "Bearer " + c.Token()
	}
	return h
}

func (c *Client) doRequest(method, path string, bodyData []byte) (map[string]interface{}, error) {
	base := c.baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := base + "/" + path
	var body io.Reader
	if bodyData != nil {
		body = bytes.NewReader(bodyData)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range c.makeHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respData, &result)
	return result, nil
}

func (c *Client) GetQRCode() (string, string, error) {
	result, err := c.doRequest("GET", "ilink/bot/get_bot_qrcode?bot_type=3", nil)
	if err != nil {
		return "", "", err
	}
	qrcode, _ := result["qrcode"].(string)
	qrcodeImg, _ := result["qrcode_img_content"].(string)
	return qrcode, qrcodeImg, nil
}

func (c *Client) CheckQRCodeStatus(qrcode string) (bool, string, string, error) {
	result, err := c.doRequest("GET", "ilink/bot/get_qrcode_status?qrcode="+qrcode, nil)
	if err != nil {
		return false, "", "", err
	}
	status, _ := result["status"].(string)
	if status == "confirmed" {
		token, _ := result["bot_token"].(string)
		baseURL, _ := result["baseurl"].(string)
		return true, token, baseURL, nil
	}
	return false, "", "", nil
}

func (c *Client) SendMessage(toID, contextToken, text string) error {
	clientID := fmt.Sprintf("cc-go-%08x", rand.Uint32())
	body, _ := json.Marshal(map[string]interface{}{
		"msg": map[string]interface{}{
			"from_user_id":  "",
			"to_user_id":    toID,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"context_token": contextToken,
			"item_list": []map[string]interface{}{
				{"type": 1, "text_item": map[string]string{"text": text}},
			},
		},
		"base_info": map[string]string{"channel_version": "1.0.2"},
	})
	_, err := c.doRequest("POST", "ilink/bot/sendmessage", body)
	return err
}

func (c *Client) SendTyping(toID, contextToken string, typing bool) error {
	ticket, err := c.getTypingTicket(toID, contextToken)
	if err != nil || ticket == "" {
		return err
	}
	status := 2
	if typing {
		status = 1
	}
	body, _ := json.Marshal(map[string]interface{}{
		"ilink_user_id": toID,
		"typing_ticket": ticket,
		"status":        status,
	})
	_, err = c.doRequest("POST", "ilink/bot/sendtyping", body)
	return err
}

func (c *Client) getTypingTicket(userID, contextToken string) (string, error) {
	c.mu.RLock()
	ticket, ok := c.typingTickets[userID]
	c.mu.RUnlock()
	if ok {
		return ticket, nil
	}
	body, _ := json.Marshal(map[string]interface{}{
		"ilink_user_id": userID,
		"context_token": contextToken,
		"base_info":     map[string]string{"channel_version": "1.0.2"},
	})
	result, err := c.doRequest("POST", "ilink/bot/getconfig", body)
	if err != nil {
		return "", err
	}
	ticket, _ = result["typing_ticket"].(string)
	if ticket != "" {
		c.mu.Lock()
		c.typingTickets[userID] = ticket
		c.mu.Unlock()
	}
	return ticket, nil
}

func (c *Client) PollMessages(timeoutMs int) ([]Message, string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"get_updates_buf": c.getUpdatesBuf,
		"base_info":       map[string]string{"channel_version": "1.0.2"},
	})
	result, err := c.doRequest("POST", "ilink/bot/getupdates", body)
	if err != nil {
		return nil, "", err
	}
	buf, _ := result["get_updates_buf"].(string)
	rawMsgs, _ := result["msgs"].([]interface{})
	var msgs []Message
	for _, raw := range rawMsgs {
		rm, _ := raw.(map[string]interface{})
		msg := parseMessage(rm)
		msgs = append(msgs, msg)
	}
	return msgs, buf, nil
}

func (c *Client) Start() {
	c.Stop()
	c.mu.Lock()
	c.stopCh = make(chan struct{})
	c.done = make(chan struct{})
	c.stopOnce = sync.Once{}
	c.mu.Unlock()
	c.SetStatus(StatusConnected)
	go c.pollLoop()
}

func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
	}
	c.SetStatus(StatusDisconnected)
}

func (c *Client) pollLoop() {
	defer close(c.done)
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}
		if c.Status() != StatusConnected {
			time.Sleep(2 * time.Second)
			continue
		}
		msgs, newBuf, err := c.PollMessages(35000)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if newBuf != "" {
			c.getUpdatesBuf = newBuf
		}
		for _, msg := range msgs {
			if msg.MessageType != 1 {
				continue
			}
			c.mu.Lock()
			c.lastContact = ContactInfo{FromID: msg.FromUserID, ContextToken: msg.ContextToken}
				onContact := c.onContact
			c.mu.Unlock()
			if onContact != nil {
				onContact(c.lastContact)
			}
			select {
			case c.msgCh <- msg:
			case <-c.stopCh:
				return
			}
		}
	}
}

func ParseLoginTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}