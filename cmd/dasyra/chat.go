package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"dasyra/internal/app"
	"dasyra/internal/arch/boot"
	"dasyra/internal/dom"
)

var escaped = regexp.MustCompile(`\\(.)`)

// chat sends one message as the owner and prints what Dasyra sends back, with a fake Telegram in place of the
// real one: a live check on a DB copy without a test bot. The text comes from the arguments, or stdin for "-".
// -as sends it as another user and -at sets the clock (server time), to replay a moment from a tester's chat.
func chat(ctx context.Context, a *boot.App) error {
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	as := flags.Int64("as", dom.OwnerID, "user id to write as")
	at := flags.String("at", "", "server time to replay at, 2006-01-02 15:04")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *at != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04", *at, time.Local)
		if err != nil {
			return err
		}
		app.SetClock(t)
	}
	text := strings.Join(flags.Args(), " ")
	if text == "-" {
		in, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		text = string(in)
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("usage: dasyra chat <message> (or - to read it from stdin)")
	}
	user, err := a.Users.Get(ctx, *as)
	if err != nil {
		return err
	}
	if user == nil || user.ChatID == 0 {
		return errors.New("the user has no chat yet: use a copy of a database the bot already ran on")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	go http.Serve(listener, http.HandlerFunc(fakeTelegram))
	a.Telegram.BaseURL = "http://" + listener.Addr().String()

	// Telegram message ids are small, a timestamp never meets a real one
	in := app.Incoming{ChatID: user.ChatID, MessageID: time.Now().Unix(), Private: true, FirstName: user.Name, Text: text}
	if err := a.Inbox.Receive(ctx, in); err != nil {
		return err
	}
	a.Work.Wait()
	return nil
}

// fakeTelegram prints what would go to the chat and answers every call with ok.
func fakeTelegram(w http.ResponseWriter, r *http.Request) {
	var params map[string]any
	_ = json.NewDecoder(r.Body).Decode(&params)
	switch path.Base(r.URL.Path) {
	case "sendMessage":
		fmt.Printf("\n--- Dasyra ---\n%s\n", escaped.ReplaceAllString(fmt.Sprint(params["text"]), "$1"))
	case "setMessageReaction":
		fmt.Printf("\n--- Dasyra reacts --- %v\n", params["reaction"])
	}
	_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
}
