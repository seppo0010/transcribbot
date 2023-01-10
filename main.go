package main

import (
	"encoding/json"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"

	"net/http"
	"os"
	"os/exec"

	vosk "github.com/alphacep/vosk-api/go"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

const SAMPLE_RATE = 16000

var token string
var model *vosk.VoskModel
var noAudio = fmt.Errorf("no audio")

func init() {
	token = os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("unable to get bot token, please set TELEGRAM_BOT_TOKEN")
	}
	var err error
	modelPath := "model"
	if os.Getenv("VOSK_MODEL_PATH") != "" {
		modelPath = os.Getenv("VOSK_MODEL_PATH")
	}
	model, err = vosk.NewModel(modelPath)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("unable to load model")
	}
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stderr)
	log.SetLevel(log.WarnLevel)
}

func getFile(bot *tgbotapi.BotAPI, message *tgbotapi.Message) (*tgbotapi.File, error) {
	if message.Audio != nil {
		f, err := bot.GetFile(tgbotapi.FileConfig{FileID: message.Audio.FileID})
		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Error("unable to get audio message")
			return nil, err
		}
		return &f, nil
	}
	if message.Voice == nil {
		return nil, noAudio
	}
	f, err := bot.GetFile(tgbotapi.FileConfig{FileID: message.Voice.FileID})
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("unable to get audio file")
		return nil, err
	}
	return &f, nil
}

func getMessageReply(bot *tgbotapi.BotAPI, message *tgbotapi.Message) (string, error) {
	f, err := getFile(bot, message)
	if err != nil {
		return "", err
	}
	link := f.Link(token)
	resp, err := http.Get(link)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("unable to download audio file")
		return "", err
	}
	defer resp.Body.Close()

	resampled, err := resample(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Warn("unable to resample audio")
		return "", err
	}
	text, err := transcribe(resampled)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("unable to transcribe audio")
		return "", err
	}
	return text, nil
}

func main() {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("unable to start telegram bot")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	log.Info("ready to receive messages")
	for update := range updates {
		if update.Message == nil {
			continue
		}
		reply, err := getMessageReply(bot, update.Message)
		if err != nil {
			if err == noAudio {
				continue
			}
			reply = "failed to transcribe audio"
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, reply)
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)
	}
}

func resample(reader io.Reader) (io.ReadCloser, error) {
	cmd := exec.Command("ffmpeg", "-nostdin", "-loglevel", "quiet", "-i", "-", "-ar", fmt.Sprintf("%v", SAMPLE_RATE), "-ac", "1", "-f", "s16le", "-")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer in.Close()
		io.Copy(in, reader)
	}()
	go cmd.Run()
	return out, nil
}

func transcribe(reader io.Reader) (string, error) {
	rec, err := vosk.NewRecognizer(model, SAMPLE_RATE)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("failed to create recognizer")
		return "", err
	}
	rec.SetWords(1)

	buf, err := io.ReadAll(reader)
	rec.AcceptWaveform(buf)
	res := struct {
		Text string `json:"text"`
	}{}
	err = json.Unmarshal([]byte(rec.FinalResult()), &res)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("failed to get recognizer final result")
		return "", err
	}
	return res.Text, nil
}
