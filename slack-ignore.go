package slack_notify_mute

import (
	"net/url"
	"bytes"
	"io/ioutil"
	"net/http"
	log "github.com/sirupsen/logrus"
	"encoding/json"
	"github.com/dgraph-io/badger"
	"crypto/sha256"
	"time"
	"fmt"
)

type SlackMessageAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool `json:"short"`
}

type SlackMessageAttachmentAction struct {
	Name string `json:"name"`
	Text string `json:"text"`
	Type string `json:"type"`
	Value string `json:"value"`
}

type SlackMessageAttachment struct {
	Title string `json:"title,omitempty"`
	Fields []SlackMessageAttachmentField `json:"fields,omitempty"`
	Fallback string `json:"fallback"`
	CallbackId string `json:"callback_id"`
	Color string `json:"color"`
	AttachmentType string `json:"attachment_type"`
	Actions []SlackMessageAttachmentAction `json:"actions"`
}

type SlackMessage struct {
	Text        string `json:"text"`
	Attachments []SlackMessageAttachment `json:"attachments"`
}

type Message struct {
	Key interface{}
	Message []byte
}

type SlackConfig struct {
	Url *url.URL
	DefaultSnooze time.Duration
}


func sendMessageToSlack(message Message, config SlackConfig) error {
	messageBytes, _ := json.Marshal(message.Key)
	shortKey, err := shortenKey(message)
	if err != nil {
		log.Fatal(err)
	}
	slackMessage := SlackMessage{
		Text: string(message.Message),
		Attachments: []SlackMessageAttachment{
			{
				Title: "You will be periodically reminded of this vulnerability.",
				Fallback: "Unable to mute",
				CallbackId: string(messageBytes[:]),
				Color: "#3AA3E3",
				AttachmentType: "default",
				Actions: []SlackMessageAttachmentAction{
					{
						Name: "mute",
						Text: "Mute",
						Type: "button",
						Value: string(shortKey[:]),
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(&slackMessage)
	client := &http.Client{}
	req, err := http.NewRequest("POST", config.Url.String(), bytes.NewReader(bodyBytes))
	if err != nil{
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

func SendMessage(message Message, config SlackConfig) (bool, error) {
	if shouldSend, err := checkShouldSend(message,Kv); err == nil && !shouldSend {
		return false, nil
	}else if err != nil {
		return false, err
	}
	if err := sendMessageToSlack(message, config); err != nil {
		return false, err
	}
	setSnooze(message, config.DefaultSnooze)
	return true, nil
}


func shortenKey(message Message) ([]byte, error) {
	keyBytes, err := json.Marshal(message.Key)
	if err != nil {
		return nil, err
	}
	keyBytesShort := sha256.Sum256(keyBytes)
	return keyBytesShort[:], nil
}

var Kv *badger.KV = GetKV(&badger.DefaultOptions)

func GetKV(opt *badger.Options) *badger.KV {
	if opt == nil {
		opt = &badger.DefaultOptions
	}

	dir, _ := ioutil.TempDir("", "badger")
	opt.Dir = dir
	opt.ValueDir = dir
	// TODO Error handling
	kv, _ := badger.NewKV(opt)
	return kv
}

func checkShouldSend(message Message, kv *badger.KV) (bool, error) {
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
	}else {
		return false, nil
	}
}

func setSnooze(message Message, duration time.Duration) error {
	key, _ := shortenKey(message)
	timeStamp, err := time.Now().Add(duration).GobEncode()
	if err != nil {
		return err
	}
	err = Kv.Set(key, timeStamp, 0x01)
	if err != nil {
		return err
	}
	return nil
}
func setMute(shortKey []byte) error {
	err := Kv.Set(shortKey, []byte("y"), 0x00)
	if err != nil {
		return err
	}
	return nil
}


func handler(w http.ResponseWriter, r *http.Request) {
	requestObject := make(map[string]interface{})
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error(err)
		return
	}
	if err := json.Unmarshal(bodyBytes, requestObject); err != nil {
		log.Error(err)
		return
	}
	defer r.Body.Close()
	for _, action := range requestObject["actions"].([]map[string]string) {
		if action["name"] == "mute" {
			setMute([]byte(action["value"]))
		}
	}

	fmt.Fprintf(w, "Request executed")
}

func StartServer() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}