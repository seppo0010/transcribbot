package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	speech "cloud.google.com/go/speech/apiv1"
	"golang.org/x/net/context"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
	"gopkg.in/telegram-bot-api.v4"
)

var token string

func init() {
	token = os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		panic("unable to get bot token, please set TELEGRAM_BOT_TOKEN")
	}
}

func getFile(bot *tgbotapi.BotAPI, message *tgbotapi.Message) (*tgbotapi.File, error) {
	if message.Audio != nil {
		if message.Audio.Duration >= 60 {
			return nil, fmt.Errorf("too long")
		}
		f, err := bot.GetFile(tgbotapi.FileConfig{FileID: message.Audio.FileID})
		return &f, err
	}
	if message.Voice == nil {
		return nil, fmt.Errorf("no audio")
	}
	if message.Voice.Duration >= 60 {
		return nil, fmt.Errorf("too long")
	}
	f, err := bot.GetFile(tgbotapi.FileConfig{FileID: message.Voice.FileID})
	return &f, err
}

func getMessageReply(bot *tgbotapi.BotAPI, message *tgbotapi.Message) (string, error) {
	f, err := getFile(bot, message)
	if err != nil {
		return "", err
	}
	link := f.Link(token)
	filename := "audio.ogg"
	out, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer out.Close()

	resp, err := http.Get(link)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}
	text, err := transcribe(filename)
	if err != nil {
		log.Println(err.Error())
		return "", err
	}
	return text, nil
}

func main() {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	log.Println("ready to receive messages")
	for update := range updates {
		if update.Message == nil {
			continue
		}
		reply, err := getMessageReply(bot, update.Message)
		if err != nil {
			reply = "failed: " + err.Error()
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, reply)
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)
	}
}

func transcribe(filename string) (string, error) {
	ctx := context.Background()

	client, err := speech.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create client")
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}

	resp, err := client.Recognize(ctx, &speechpb.RecognizeRequest{
		Config: &speechpb.RecognitionConfig{
			Encoding:        speechpb.RecognitionConfig_OGG_OPUS,
			SampleRateHertz: 16000,
			LanguageCode:    "es-AR",
		},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: data},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to recognize")
	}

	for _, result := range resp.Results {
		for _, alt := range result.Alternatives {
			return fmt.Sprintf("\"%v\" (confidence=%3f)\n", alt.Transcript, alt.Confidence), nil
		}
	}
	return "", fmt.Errorf("unable to transcribe audio")
}
