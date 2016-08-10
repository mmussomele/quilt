package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"
)

// Sleep is stored in a variable so it can be mocked out in the unit tests
var Sleep = time.Sleep

// WaitFor simply waits until `pred` returns true. If it times out before `pred` returns
// true, it will return an error.
func WaitFor(pred func() bool, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	for {
		select {
		case <-timeoutChan:
			return errors.New("timed out")
		default:
			if pred() {
				return nil
			}
		}
		Sleep(time.Second)
	}
}

type message struct {
	Title string `json:"title"`
	Short bool   `json:"short"`
	Value string `json:"value"`
}

// SlackPost contains information needed to post to slack.
type SlackPost struct {
	Channel   string    `json:"channel"`
	Color     string    `json:"color"`
	Fields    []message `json:"fields"`
	Pretext   string    `json:"pretext"`
	Username  string    `json:"username"`
	Iconemoji string    `json:"icon_emoji"`
}

// ToPost transforms the given parameters into a SlackPost struct.
func ToPost(failed bool, channel, pretext, text string) SlackPost {
	iconemoji := ":confetti_ball:"
	color := "#009900" // Green
	if failed {
		iconemoji = ":oncoming_police_car:"
		color = "#D00000" // Red
	}

	return SlackPost{
		Channel:   channel,
		Color:     color,
		Pretext:   pretext,
		Username:  "quilt-bot",
		Iconemoji: iconemoji,
		Fields: []message{
			{
				Title: "Continuous Integration",
				Short: false,
				Value: text,
			},
		},
	}
}

// Slack posts the given SlackPost to slack using the given slack hookurl.
func Slack(hookurl string, p SlackPost) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}

	resp, err := http.Post(hookurl, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t, _ := ioutil.ReadAll(resp.Body)
		return errors.New(string(t))
	}

	return nil
}
