package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	rcon "github.com/gtaylor/factorio-rcon"
	"github.com/pkg/errors"
)

type Users map[string]bool

func GetUsers(addr, pass string) (Users, error) {
	r, err := rcon.Dial(addr)
	if err != nil {
		return nil, errors.Wrapf(err, "NewConnection %v", addr)
	}
	err = r.Authenticate(pass)
	if err != nil {
		return nil, errors.Wrapf(err, "new connection %v", addr)
	}

	players, err := r.CmdPlayers()
	if err != nil {
		return nil, err
	}

	users := Users{}
	for _, x := range players {
		users[x.Name] = x.Online
	}

	return users, nil
}

func (old Users) Cmp(new Users) []string {
	type change struct{ was, is bool }

	merged := map[string]change{}
	for username, online := range old {
		merged[username] = change{online, false}
	}

	for username, online := range new {
		ch, ok := merged[username]
		if !ok {
			ch = change{false, false}
		}
		ch.is = online

		merged[username] = ch
	}

	messages := []string{}
	for username, change := range merged {
		if change.is == change.was {
			continue
		}

		if change.is && change.was == false {
			messages = append(messages, fmt.Sprintf("%v logged in", username))
		}

		if change.was && change.is == false {
			messages = append(messages, fmt.Sprintf("%v logged off", username))
		}
	}

	return messages
}

func Post(endpoint, message string) error {
	body := struct {
		Content string `json:"content"`
	}{
		Content: message,
	}

	text, err := json.Marshal(body)
	if err != nil {
		return err
	}

	log.Printf("change msg sent %v", message)
	_, err = http.Post(endpoint, "application/json", bytes.NewBuffer(text))
	return err
}

func main() {
	var (
		addr = flag.String("addr", "", "the rcon address")
		pass = flag.String("pass", "", "the rcon password")
		hook = flag.String("hook", "", "the hook url")
	)
	flag.Parse()

	state, err := GetUsers(*addr, *pass)
	if err != nil {
		panic(err) //todo anything but this
	}

	for {
		// wait
		time.Sleep(5 * time.Second)

		// get the state
		new, err := GetUsers(*addr, *pass)
		if err != nil {
			panic(err)
		}

		for _, change := range state.Cmp(new) {
			err = Post(*hook, change)
			if err != nil {
				panic(err)
			}
		}

		// set state to new
		state = new
	}
}
