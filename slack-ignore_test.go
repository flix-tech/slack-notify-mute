package slack_notify_mute

import (
	"encoding/hex"
	"github.com/dgraph-io/badger"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

var testBadgerPrefix string = "test"

func TestPrepareRequest(t *testing.T) {
	message := &Message{
		Message: []byte("Foo"),
		Key:     "Foo",
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
	message := &Message{
		Message: []byte("Foo"),
		Key:     "Bar",
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
	file, err := os.Open("test_resources/samplerequest.txt")
	defer file.Close()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/", file)
	request.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	webhookBody, err := parseWebhook(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(webhookBody.Actions) != 1 {
		t.Fatal("WebhookBody Actions count did not match")
	}
}

func TestMessageNotSent(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", testBadgerPrefix)
	kv, err := GetKV(&badger.DefaultOptions, tempDir)
	defer kv.Close()
	if err != nil {
		t.Fatal(err)
	}
	message := &Message{
		Message: []byte("Foo"),
		Key:     "Bar",
	}
	if shouldSend, err := checkShouldSend(message, kv); err != nil {
		t.Fatal(err)
	} else if !shouldSend {
		t.Fatal("Expected to send message")
	}
}

func TestMessageWasSnoozed(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", testBadgerPrefix)
	kv, err := GetKV(&badger.DefaultOptions, tempDir)
	defer kv.Close()
	if err != nil {
		t.Fatal(err)
	}
	message := &Message{
		Message: []byte("Foo"),
		Key:     "Bar",
	}
	if shortKey, err := shortenKey(message); err != nil {
		t.Fatal(err)
	} else {
		setSnooze(shortKey, kv, 1*time.Second)
	}
	if shouldSend, err := checkShouldSend(message, kv); err != nil {
		t.Fatal(err)
	} else if shouldSend {
		t.Fatal("Expected to not send message")
	}
	time.Sleep(2 * time.Second)
	if shouldSend, err := checkShouldSend(message, kv); err != nil {
		t.Fatal(err)
	} else if !shouldSend {
		t.Fatal("Expected to send message")
	}
}

func TestMessageWasMuted(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", testBadgerPrefix)
	kv, err := GetKV(&badger.DefaultOptions, tempDir)
	defer kv.Close()
	if err != nil {
		t.Fatal(err)
	}
	message := &Message{
		Message: []byte("Foo"),
		Key:     "Bar",
	}
	shortKey, _ := shortenKey(message)
	if shouldSend, err := checkShouldSend(message, kv); err != nil {
		t.Fatal(err)
	} else if !shouldSend {
		t.Fatal("Expected to send message")
	}
	setMute(shortKey, kv)
	if shouldSend, err := checkShouldSend(message, kv); err != nil {
		t.Fatal(err)
	} else if shouldSend {
		t.Fatal("Expected to not send message")
	}
}

func TestHandler(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", testBadgerPrefix)
	kv, err := GetKV(&badger.DefaultOptions, tempDir)
	defer kv.Close()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("test_resources/samplerequest.txt")
	if err != nil {
		t.Fatal(err)
	}
	handler := createHandler(kv)
	request := httptest.NewRequest("POST", "/", file)
	request.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	w := httptest.NewRecorder()
	handler(w, request)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Error("Unexpected status code: " + strconv.Itoa(resp.StatusCode))
	}
}
