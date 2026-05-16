package wechat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const DefaultBaseURL = "https://ilinkai.weixin.qq.com"
const DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

const (
	MediaTypeImage = 1
	MediaTypeVideo = 2
	MediaTypeFile  = 3
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusExpired      Status = "expired"
)

type Client struct {
	baseURL         string
	botToken        string
	loginTime       time.Time
	cdnBaseURL      string
	status          Status
	mu              sync.RWMutex
	httpClient      *http.Client
	cdnHttpClient   *http.Client
	msgCh           chan Message
	getUpdatesBuf   string
	lastContact     ContactInfo
	typingTickets   map[string]string
	stopCh          chan struct{}
	done            chan struct{}
	stopOnce        sync.Once
	reconnectStopCh chan struct{}
	onContact       func(ContactInfo)
	onTokenUpdate   func(token, baseURL string)
}

type ContactInfo struct {
	FromID       string
	ContextToken string
}

func NewClient(baseURL, botToken string, loginTime time.Time, cdnBaseURL string) *Client {
	if cdnBaseURL == "" {
		cdnBaseURL = DefaultCDNBaseURL
	}
	return &Client{
		baseURL:       baseURL,
		botToken:      botToken,
		loginTime:     loginTime,
		cdnBaseURL:    cdnBaseURL,
		status:        StatusDisconnected,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		cdnHttpClient: &http.Client{Timeout: 5 * time.Minute},
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

func (c *Client) SetTokenUpdateCallback(fn func(token, baseURL string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTokenUpdate = fn
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
	reqURL := base + "/" + path
	var body io.Reader
	if bodyData != nil {
		body = bytes.NewReader(bodyData)
	}
	req, err := http.NewRequest(method, reqURL, body)
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
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("invalid json response: %w", err)
	}
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

// SendMessageItemList sends a message with a custom item_list.
func (c *Client) SendMessageItemList(toID, contextToken string, itemList []map[string]interface{}) error {
	clientID := fmt.Sprintf("cc-go-%08x", rand.Uint32())
	body, _ := json.Marshal(map[string]interface{}{
		"msg": map[string]interface{}{
			"from_user_id":  "",
			"to_user_id":    toID,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"context_token": contextToken,
			"item_list":     itemList,
		},
		"base_info": map[string]string{"channel_version": "1.0.2"},
	})
	_, err := c.doRequest("POST", "ilink/bot/sendmessage", body)
	return err
}

func (c *Client) SendMessage(toID, contextToken, text string) error {
	return c.SendMessageItemList(toID, contextToken, []map[string]interface{}{
		{"type": 1, "text_item": map[string]string{"text": text}},
	})
}

// --- Media upload methods ---

func (c *Client) GetUploadURL(fileKey string, mediaType int, toUserID string, rawSize int, rawFileMD5 string, fileSize int, aesKeyHex string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"filekey":        fileKey,
		"media_type":     mediaType,
		"to_user_id":     toUserID,
		"rawsize":        rawSize,
		"rawfilemd5":     rawFileMD5,
		"filesize":       fileSize,
		"no_need_thumb":  true,
		"aeskey":         aesKeyHex,
		"base_info":      map[string]string{"channel_version": "1.0.2"},
	})
	result, err := c.doRequest("POST", "ilink/bot/getuploadurl", body)
	if err != nil {
		return "", fmt.Errorf("getuploadurl: %w", err)
	}
	uploadParam, _ := result["upload_param"].(string)
	if uploadParam == "" {
		return "", fmt.Errorf("getuploadurl returned no upload_param")
	}
	return uploadParam, nil
}

func (c *Client) UploadToCDN(ciphertext []byte, uploadParam, fileKey string) (string, error) {
	cdnURL := c.cdnBaseURL + "/upload?encrypted_query_param=" + url.QueryEscape(uploadParam) + "&filekey=" + url.QueryEscape(fileKey)
	req, err := http.NewRequest("POST", cdnURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.cdnHttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cdn upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errMsg := resp.Header.Get("x-error-message")
		if errMsg == "" {
			errMsg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("cdn upload failed: %s", errMsg)
	}
	downloadParam := resp.Header.Get("x-encrypted-param")
	if downloadParam == "" {
		return "", fmt.Errorf("cdn upload response missing x-encrypted-param header")
	}
	return downloadParam, nil
}

func (c *Client) UploadFile(filePath string, mediaType int, toUserID string) (*UploadResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	rawSize := len(data)
	rawFileMD5 := fileMD5(data)
	fileSize := computeCiphertextSize(rawSize)

	rawKey, aesKeyHex, _ := generateAESKey()
	fileKey := generateFileKey()

	uploadParam, err := c.GetUploadURL(fileKey, mediaType, toUserID, rawSize, rawFileMD5, fileSize, aesKeyHex)
	if err != nil {
		return nil, err
	}

	ciphertext, err := aesEncryptECB(rawKey, data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}

	downloadParam, err := c.UploadToCDN(ciphertext, uploadParam, fileKey)
	if err != nil {
		return nil, err
	}

	return &UploadResult{
		DownloadParam:       downloadParam,
		AESKeyBase64:        aesKeyToBase64(aesKeyHex),
		AESKeyHex:           aesKeyHex,
		FileSize:            rawSize,
		FileSizeCiphertext: fileSize,
	}, nil
}

func makeCDNMedia(result *UploadResult) CDNMedia {
	return CDNMedia{
		EncryptQueryParam: result.DownloadParam,
		AESKey:            result.AESKeyBase64,
		EncryptType:       1,
	}
}

func (c *Client) SendImage(toID, contextToken, filePath string) error {
	result, err := c.UploadFile(filePath, MediaTypeImage, toID)
	if err != nil {
		return err
	}
	media := makeCDNMedia(result)
	return c.SendMessageItemList(toID, contextToken, []map[string]interface{}{
		{"type": 2, "image_item": map[string]interface{}{
			"media":    media,
			"mid_size": result.FileSizeCiphertext,
		}},
	})
}

func (c *Client) SendFile(toID, contextToken, filePath string) error {
	result, err := c.UploadFile(filePath, MediaTypeFile, toID)
	if err != nil {
		return err
	}
	media := makeCDNMedia(result)
	fileName := filepath.Base(filePath)
	return c.SendMessageItemList(toID, contextToken, []map[string]interface{}{
		{"type": 4, "file_item": map[string]interface{}{
			"media":     media,
			"file_name": fileName,
			"len":       fmt.Sprintf("%d", result.FileSize),
		}},
	})
}

func (c *Client) SendVideo(toID, contextToken, filePath string) error {
	result, err := c.UploadFile(filePath, MediaTypeVideo, toID)
	if err != nil {
		return err
	}
	media := makeCDNMedia(result)
	return c.SendMessageItemList(toID, contextToken, []map[string]interface{}{
		{"type": 5, "video_item": map[string]interface{}{
			"media":      media,
			"video_size": result.FileSizeCiphertext,
		}},
	})
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
	c.mu.RLock()
	buf := c.getUpdatesBuf
	c.mu.RUnlock()
	body, _ := json.Marshal(map[string]interface{}{
		"get_updates_buf": buf,
		"base_info":       map[string]string{"channel_version": "1.0.2"},
	})
	result, err := c.doRequest("POST", "ilink/bot/getupdates", body)
	if err != nil {
		return nil, "", err
	}
	newBuf, _ := result["get_updates_buf"].(string)
	rawMsgs, _ := result["msgs"].([]interface{})
	var msgs []Message
	for _, raw := range rawMsgs {
		rm, _ := raw.(map[string]interface{})
		msg := parseMessage(rm)
		msgs = append(msgs, msg)
	}
	return msgs, newBuf, nil
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
			log.Printf("[wechat] poll error: %v", err)
			c.SetStatus(StatusDisconnected)
			time.Sleep(2 * time.Second)
			continue
		}
		if newBuf != "" {
			c.mu.Lock()
			c.getUpdatesBuf = newBuf
			c.mu.Unlock()
		}
		for _, msg := range msgs {
			if msg.MessageType != 1 {
				continue
			}
			contact := ContactInfo{FromID: msg.FromUserID, ContextToken: msg.ContextToken}
			c.mu.Lock()
			c.lastContact = contact
			onContact := c.onContact
			c.mu.Unlock()
			if onContact != nil {
				onContact(contact)
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