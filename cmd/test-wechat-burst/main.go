package main

import (
	"fmt"
	"log"
	"time"

	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/wechat"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Wechat.BotToken == "" {
		log.Fatal("未配置微信机器人 token，请先在 Web 界面扫码登录")
	}
	if cfg.Wechat.LastFromID == "" {
		log.Fatal("未找到最近联系人，请先在微信给机器人发一条消息")
	}

	client := wechat.NewClient(cfg.Wechat.BaseURL, cfg.Wechat.BotToken, wechat.ParseLoginTime(cfg.Wechat.LoginTime))

	toID := cfg.Wechat.LastFromID
	ctxToken := cfg.Wechat.LastContextToken

	fmt.Printf("目标用户: %s\n", toID)
	fmt.Printf("context_token: %s...\n", ctxToken[:20])
	fmt.Println("---")

	for i := 1; i <= 12; i++ {
		msg := fmt.Sprintf("测试消息 #%d", i)
		fmt.Printf("[%d/12] 发送: %s", i, msg)

		err := client.SendMessage(toID, ctxToken, msg)
		if err != nil {
			fmt.Printf("  ❌ 失败: %v\n", err)
		} else {
			fmt.Println("  ✅ 成功")
		}

		if i < 12 {
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Println("---")
	fmt.Println("测试完成。请检查微信是否收到全部 12 条消息。")
}