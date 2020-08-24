package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tomlazar/factorio-bot/logger"

	rcon "github.com/gtaylor/factorio-rcon"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Ctx is the current internals of the tcp and the rcon server
type Ctx struct {
	hook string
	rcon *rcon.RCON
}

// NewCtx sets up the server connetion and the logger
func NewCtx(addr, pass, hook string) (*Ctx, error) {
	r, err := rcon.Dial(addr)
	if err != nil {
		return nil, errors.Wrapf(err, "NewConnection %v", addr)
	}

	err = r.Authenticate(pass)
	if err != nil {
		return nil, errors.Wrapf(err, "new connection %v", addr)
	}

	return &Ctx{hook, r}, nil
}

// Users is a map of users to their current status
type Users map[string]bool

// GetUsers get the current list of users from the server
func (c *Ctx) GetUsers() (Users, error) {
	players, err := c.rcon.CmdPlayers()
	if err != nil {
		return nil, err
	}

	users := Users{}
	for _, x := range players {
		users[x.Name] = x.Online
	}

	return users, nil
}

// Close cleanly shuts down the tcp connection
func (c *Ctx) Close() error {
	return c.rcon.Close()
}

// Cmp compares the old state to the new one
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

func (c *Ctx) post(message string) error {
	body := map[string]string{
		"content": message,
	}

	text, err := json.Marshal(body)
	if err != nil {
		return err
	}

	_, err = http.Post(c.hook, "application/json", bytes.NewBuffer(text))
	return err
}

// Post tries to sent the message to discord 3 times
func (c *Ctx) Post(message string) error {
	var err error
	for i := 0; i < 3; i++ {
		err = c.post(message)
		if err == nil {
			return err
		}
	}

	return err
}

func (c *Ctx) Scan(ctx context.Context, log *zap.Logger, done chan bool) {
	state, err := c.GetUsers()
	if err != nil {
		log.Error("could not load initial state", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("main loop canceled through context")
			done <- true
			return
		default:
			log.Debug("scan loop execution")

			// wait for five seconds
			time.Sleep(5 * time.Second)

			// get the state
			new, err := c.GetUsers()
			if err != nil {
				log.Error("could not get users from the rcon server", zap.Error(err))
				continue
			}

			for _, change := range state.Cmp(new) {
				err = c.Post(change)
				if err != nil {
					log.Error("could not post changes to discord",
						zap.String("msg", change),
						zap.Error(err),
					)
					continue
				}
			}

			// set state to new
			state = new
		}
	}
}

func main() {
	var (
		addr  = flag.String("addr", "", "the rcon address")
		pass  = flag.String("pass", "", "the rcon password")
		hook  = flag.String("hook", "", "the hook url")
		debug = flag.Bool("debug", false, "add debug info")
	)
	flag.Parse()

	log, err := logger.New(*debug)
	if err != nil {
		panic(err)
	}

	ctx, err := NewCtx(*addr, *pass, *hook)
	if err != nil {
		panic(err)
	}
	defer ctx.Close()

	// create a cancel context
	context, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	done := make(chan bool)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// start the scan loop
	go ctx.Scan(context, log, done)

	// monitor for sigclose
	<-sigs
	cancel()
	<-done
}
