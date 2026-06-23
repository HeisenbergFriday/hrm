package main

import (
	"flag"
	"log"
	"strings"

	"peopleops/internal/config"
	"peopleops/internal/dingtalk"
)

func main() {
	userID := flag.String("user", "", "DingTalk user_id that will receive the test message")
	title := flag.String("title", "绩效通知联调", "message title")
	content := flag.String("content", "这是一条钉钉真实环境联调消息。如收到，说明企业内部消息链路可用。", "message content")
	button := flag.String("button", "打开系统", "action card button text")
	actionURL := flag.String("url", "", "action card URL; falls back to configured app home URL when empty")
	textOnly := flag.Bool("text", false, "send a text message instead of an action card")
	flag.Parse()

	recipient := strings.TrimSpace(*userID)
	if recipient == "" {
		log.Fatal("missing -user; refuse to send without an explicit test recipient")
	}

	if err := config.Load(); err != nil {
		log.Printf("load env warning: %v", err)
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("dingtalk init failed: %v", err)
	}

	url := strings.TrimSpace(*actionURL)
	if url == "" {
		url = dingtalk.GetConfiguredAppHomeURL()
	}

	var err error
	if *textOnly || url == "" {
		log.Printf("sending dingtalk text message to user %s", recipient)
		err = dingtalk.SendCorpMessageToUser(recipient, *title, *content)
	} else {
		log.Printf("sending dingtalk action card to user %s", recipient)
		err = dingtalk.SendCorpActionCardToUser(recipient, *title, *content, *button, url)
	}
	if err != nil {
		log.Fatalf("send dingtalk message failed: %v", err)
	}

	log.Printf("dingtalk message accepted for user %s", recipient)
}
