package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var (
	discordPostErrors = make(chan string, 128)
)

func routineDiscordErrorReporter() {
	lastMessage := time.Now()
	lastAggregate := ""
	dflusher := time.NewTicker(time.Second * 5)
	for {
		select {
		case msg := <-discordPostErrors:
			lastAggregate += msg + "\n"
			if time.Since(lastMessage).Seconds() > 2.5*float64(time.Second) {
				discordSendError(lastAggregate)
				lastAggregate = ""
				lastMessage = time.Now()
			}
		case <-dflusher.C:
			if lastAggregate == "" {
				continue
			}
			discordSendError(lastAggregate)
			lastAggregate = ""
			lastMessage = time.Now()
		}
	}
}

func discordSendError(content string) {
	webhookUrl, ok := cfg.GetString("discordErrorsWebhook")
	if !ok {
		log.Println("Errors discord webhook not set!!!")
	}
	b, err := json.Marshal(map[string]interface{}{
		"username": "Backend",
		"content":  content,
	})
	if err != nil {
		log.Println("Error sending webhook:", err)
	}
	req, err := http.NewRequest("POST", webhookUrl, bytes.NewBuffer(b))
	if err != nil {
		log.Println("Error sending webhook:", err)
	}
	req.Header.Add("Content-Type", "application/json")
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		log.Println("Error sending webhook:", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		defer resp.Body.Close()
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error sending webhook:", err)
		}
		log.Println("Error sending webhook:", responseBody)
	}
}

func discordPostError(format string, args ...any) {
	discordPostErrors <- fmt.Sprintf(format, args...)
}
