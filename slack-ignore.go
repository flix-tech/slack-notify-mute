package slack_notify_mute

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

type SlackMessageAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type SlackMessageAttachmentAction struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type SlackMessageAttachment struct {
	Title          string                         `json:"title,omitempty"`
	Fields         []SlackMessageAttachmentField  `json:"fields,omitempty"`
	Fallback       string                         `json:"fallback"`
	CallbackId     string                         `json:"callback_id"`
	Color          string                         `json:"color"`
	AttachmentType string                         `json:"attachment_type"`
	Actions        []SlackMessageAttachmentAction `json:"actions"`
}

type SlackMessage struct {
	Text        string                   `json:"text"`
	Attachments []SlackMessageAttachment `json:"attachments"`
}

type Message struct {
	Key     interface{}
	Message []byte
}

type SlackConfig struct {
	Url           *url.URL
	DefaultSnooze time.Duration
	DataDir       string
}

var lock *sync.Mutex

func sendMessageToSlack(message *Message, config SlackConfig) error {
	bodyBytes, err := prepareRequest(message)
	if err != nil {
		return err
	}
	client := &http.Client{}
	req, err := http.NewRequest("POST", config.Url.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Add("Content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	log.Warn("Response from Slack: " + string(respBodyBytes[:]))
	defer resp.Body.Close()
	return nil
}

func prepareRequest(message *Message) ([]byte, error) {
	messageBytes, err := json.Marshal(message.Key)
	if err != nil {
		return nil, err
	}
	shortKey, err := shortenKey(message)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	slackMessage := SlackMessage{
		Text: string(message.Message),
		Attachments: []SlackMessageAttachment{
			{
				Title:          "You will be periodically reminded of this vulnerability.",
				Fallback:       "Unable to mute",
				CallbackId:     string(messageBytes[:]),
				Color:          "#3AA3E3",
				AttachmentType: "default",
				Actions: []SlackMessageAttachmentAction{
					{
						Name:  "mute",
						Text:  "Mute",
						Type:  "button",
						Value: string(shortKey[:]),
					},
					{
						Name:  "snooze",
						Text:  "Snooze",
						Type:  "button",
						Value: string(shortKey[:]),
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(&slackMessage)
	return bodyBytes, nil
}

func SendMessage(message *Message, config SlackConfig) (bool, error) {
	if lock == nil {
		lock = &sync.Mutex{}
	}
	lock.Lock()
	defer lock.Unlock()
	kv, err := GetKV(&badger.DefaultOptions, config.DataDir)
	if err != nil {
		return false, err
	}
	if shouldSend, err := checkShouldSend(message, kv); err == nil && !shouldSend {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := sendMessageToSlack(message, config); err != nil {
		return false, err
	}
	shortMessage, err := shortenKey(message)
	if err != nil {
		return false, err
	}
	setSnooze(shortMessage, kv, config.DefaultSnooze)
	return true, nil
}

func shortenKey(message *Message) ([]byte, error) {
	keyBytes, err := json.Marshal(message.Key)
	if err != nil {
		return nil, err
	}
	keyBytesShort := sha256.Sum256(keyBytes)
	return keyBytesShort[:], nil
}

// is written to, when starting the server

func GetKV(opt *badger.Options, path string) (*badger.KV, error) {
	if opt == nil {
		opt = &badger.DefaultOptions
	}
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, err
	}
	opt.Dir = path
	opt.ValueDir = path
	if kv, err := badger.NewKV(opt); err != nil {
		return nil, err
	} else {
		return kv, nil
	}
}

func checkShouldSend(message *Message, kv *badger.KV) (bool, error) {
	keyBytesFixed, err := shortenKey(message)
	if err != nil {
		return false, err
	}

	var item badger.KVItem
	if err := kv.Get(keyBytesFixed[:], &item); err != nil {
		return false, err
	}
	if item.Value() == nil {
		return true, nil
	}
	if item.UserMeta() == 0x01 {
		timePoint := time.Now()
		err := timePoint.GobDecode(item.Value())
		if err != nil {
			return false, err
		}
		return time.Now().After(timePoint), nil
	} else {
		return false, nil
	}
}

func setSnooze(shortKey []byte, kv *badger.KV, duration time.Duration) error {
	timeStamp, err := time.Now().Add(duration).GobEncode()
	if err != nil {
		return err
	}
	err = kv.Set(shortKey, timeStamp, 0x01)
	if err != nil {
		return err
	}
	return nil
}
func setMute(shortKey []byte, kv *badger.KV) error {
	err := kv.Set(shortKey, []byte("y"), 0x00)
	if err != nil {
		return err
	}
	return nil
}

type WebhookBody struct {
	Actions []SlackMessageAttachmentAction `json:"actions"`
}

func parseWebhook(r *http.Request) (*WebhookBody, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, err
	}
	if len(r.Form["payload"]) == 0 {
		return nil, errors.New("No payload in body")
	}
	payloadString := r.Form["payload"][0]
	bodyBytes := []byte(payloadString)
	requestObject := &WebhookBody{}
	if err != nil {
		log.Error(err)
		return nil, err
	}
	if err := json.Unmarshal(bodyBytes, requestObject); err != nil {
		log.Print(string(bodyBytes))
		log.Error(err)
		return nil, err
	}
	return requestObject, nil
}

func createHandler(kv *badger.KV) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestObject, err := parseWebhook(r)
		defer r.Body.Close()
		if err != nil {
			w.WriteHeader(400)
			return
		}
		for _, action := range requestObject.Actions {
			if action.Name == "mute" {
				setMute([]byte(action.Value), kv)
			} else if action.Name == "snooze" {
				setSnooze([]byte(action.Value), kv, 30*24*time.Hour)
			}
		}

		fmt.Fprintf(w, "Request executed")
	}
}

func StartServer() {
	kv, err := GetKV(&badger.DefaultOptions, "badger")
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", createHandler(kv))
	http.ListenAndServe(":8080", nil)
}
