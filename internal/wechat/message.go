package wechat

type Message struct {
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	MessageType  int    `json:"message_type"`
	MessageState int    `json:"message_state"`
	ContextToken string `json:"context_token"`
	Text         string `json:"text"`
}

func parseMessage(raw map[string]interface{}) Message {
	msg := Message{}
	msg.FromUserID, _ = raw["from_user_id"].(string)
	msg.ToUserID, _ = raw["to_user_id"].(string)
	if mt, ok := raw["message_type"].(float64); ok {
		msg.MessageType = int(mt)
	}
	if ms, ok := raw["message_state"].(float64); ok {
		msg.MessageState = int(ms)
	}
	msg.ContextToken, _ = raw["context_token"].(string)
	items, _ := raw["item_list"].([]interface{})
	for _, item := range items {
		im, _ := item.(map[string]interface{})
		t, _ := im["type"].(float64)
		if t == 1 {
			ti, _ := im["text_item"].(map[string]interface{})
			msg.Text, _ = ti["text"].(string)
			break
		}
	}
	return msg
}