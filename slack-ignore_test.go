package slack_notify_mute

import (
	"testing"
	"encoding/hex"
	"os"
	"github.com/dgraph-io/badger"
	"time"
)

var badgerPrefix string = "test"

func TestPrepareRequest(t *testing.T) {
	message := &Message{
		Message: []byte("Foo"),
		Key: "Foo",
	}
	requestBytes, err := prepareRequest(message)
	if err != nil {
		t.Fatal(err)
	}
	if len(requestBytes) < 50 {
		t.Fatal("requestBytes was too small")
	}
}

func TestShortenKey(t *testing.T) {
	message:= &Message{
		Message: []byte("Foo"),
		Key: "Bar",
	}
	shortKey, err := shortenKey(message)
	if err != nil {
		t.Fatal(err)
	}
	expected := "9cf3754f15467c507012911cc590ee7a571bdb4c6bba30c605868304033db330"
	if actual := hex.EncodeToString(shortKey); actual != expected {
		t.Fatal("Hashes did not match, Expected:", expected, "Actual:", actual)
	}
}

func TestParseWebhookBody(t *testing.T) {
	file, err := os.Open("samplewebhook.json")
	if err != nil {
		t.Fatal(err)
	}
	webhookBody, err := parseWebhookBody(file)
	if err != nil{
		t.Fatal(err)
	}
	if len(webhookBody.Actions) != 1 {
		t.Fatal("WebhookBody Actions count did not match")
	}
}

func TestMessageNotSent(t *testing.T) {
	kv, err := GetKV(&badger.DefaultOptions, badgerPrefix)
	if err != nil {
		t.Fatal(err)
	}
	message:= &Message{
		Message: []byte("Foo"),
		Key: "Bar",
	}
	if shouldSend, err := checkShouldSend(message, kv); err != nil{
		t.Fatal(err)
	}else if !shouldSend{
		t.Fatal("Expected to send message")
	}
}

func TestMessageWasSnoozed(t *testing.T) {
	kv, err := GetKV(&badger.DefaultOptions, badgerPrefix)
	if err != nil {
		t.Fatal(err)
	}
	message:= &Message{
		Message: []byte("Foo"),
		Key: "Bar",
	}
	if shortKey, err := shortenKey(message); err != nil {
		t.Fatal(err)
	}else{
		setSnooze(shortKey,kv, 1* time.Second)
	}
	if shouldSend, err := checkShouldSend(message, kv); err != nil{
		t.Fatal(err)
	}else if shouldSend{
		t.Fatal("Expected to not send message")
	}
	time.Sleep(2 * time.Second)
	if shouldSend, err := checkShouldSend(message, kv); err != nil{
		t.Fatal(err)
	}else if !shouldSend{
		t.Fatal("Expected to send message")
	}
}

func TestMessageWasMuted(t *testing.T) {
	kv, err := GetKV(&badger.DefaultOptions, badgerPrefix)
	if err != nil {
		t.Fatal(err)
	}
	message:= &Message{
		Message: []byte("Foo"),
		Key: "Bar",
	}
	shortKey, _ := shortenKey(message)
	if shouldSend, err := checkShouldSend(message, kv); err != nil{
		t.Fatal(err)
	}else if !shouldSend{
		t.Fatal("Expected to send message")
	}
	setMute(shortKey,kv)
	if shouldSend, err := checkShouldSend(message, kv); err != nil{
		t.Fatal(err)
	}else if shouldSend{
		t.Fatal("Expected to not send message")
	}
}