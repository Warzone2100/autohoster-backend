package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"
)

var (
	discordPostErrors = make(chan string, 128)
)

func routineDiscordErrorReporter() {
	// lastMessage := time.Now()
	lastAggregate := ""
	dflusher := time.NewTicker(time.Second * 60)
	for {
		select {
		case msg := <-discordPostErrors:
			lastAggregate += msg + "\n"
			// if time.Since(lastMessage).Seconds() > 2.5*float64(time.Second) {
			// 	discordSendError(lastAggregate)
			// 	lastAggregate = ""
			// 	lastMessage = time.Now()
			// }
		case <-dflusher.C:
			if lastAggregate == "" {
				continue
			}
			discordSendError(lastAggregate)
			lastAggregate = ""
			// lastMessage = time.Now()
		}
	}
}

func discordSendError(content string) {
	webhookUrl, ok := cfg.GetString("discordErrorsWebhook")
	if !ok {
		log.Println("Errors discord webhook not set!!!")
		return
	}
	if len(content) < 1995 {
		discordSendErrorWithContent(webhookUrl, content)
	} else {
		discordSendErrorWithFile(webhookUrl, content)
	}
}

func discordSendErrorWithContent(webhookUrl, content string) {
	payload_json, err := json.Marshal(map[string]interface{}{
		"username": "Backend",
		"content":  content,
	})
	if err != nil {
		log.Println("Error marshling webhook json payload:", err)
		return
	}
	req, err := http.NewRequest("POST", webhookUrl, bytes.NewBuffer(payload_json))
	if err != nil {
		log.Println("Error creating webhook request:", err)
		return
	}
	req.Header.Add("Content-Type", "application/json")
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		log.Println("Error sending webhook:", err)
		return
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

func discordSendErrorWithFile(webhookUrl, content string) {
	payload_json, err := json.Marshal(map[string]interface{}{
		"username": "Backend",
		"attachments": []map[string]any{{
			"id":          0,
			"description": "big content",
			"filename":    "msg.txt",
		}},
	})
	if err != nil {
		log.Println("Error marshling webhook json:", err)
		return
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	p1w, err := w.CreateFormField("payload_json")
	if err != nil {
		log.Println("Error creating webhook json multipart:", err)
		return
	}
	_, err = p1w.Write(payload_json)
	if err != nil {
		log.Println("Error writing webhook json multipart:", err)
		return
	}
	p2w, err := w.CreateFormFile("file[0]", "msg.txt")
	if err != nil {
		log.Println("Error creating webhook content multipart:", err)
		return
	}
	_, err = p2w.Write([]byte(content))
	if err != nil {
		log.Println("Error writing webhook content multipart:", err)
		return
	}
	w.Close()

	req, err := http.NewRequest("POST", webhookUrl, &b)
	if err != nil {
		log.Println("Error creating webhook request:", err)
		return
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	c := http.Client{Timeout: 5 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		log.Println("Error sending webhook request:", err)
		return
	}
	if res.StatusCode != http.StatusOK {
		rspb, err := io.ReadAll(res.Body)
		if err != nil {
			log.Println("Error reading discord's webhook response: ", err)
		}
		log.Println("Discord returned not 200: ", res.StatusCode, string(rspb))
	}
}

func discordPostError(format string, args ...any) {
	discordPostErrors <- fmt.Sprintf(format, args...)
}
