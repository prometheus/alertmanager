// Copyright 2009 Thomas Jager <mail@jager.no>  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package irc

import (
	"crypto/tls"
	"log"
	"net"
	"sync"
	"time"
)

type Connection struct {
	sync.WaitGroup
	Debug        bool
	Error        chan error
	Password     string
	UseTLS       bool
	UseSASL      bool
	SASLLogin    string
	SASLPassword string
	SASLMech     string
	TLSConfig    *tls.Config
	Version      string
	Timeout      time.Duration
	PingFreq     time.Duration
	KeepAlive    time.Duration
	Server       string

	socket net.Conn
	pwrite chan string
	end    chan struct{}

	nick        string //The nickname we want.
	nickcurrent string //The nickname we currently have.
	user        string
	registered  bool
	events      map[string]map[int]func(*Event)

	QuitMessage string
	lastMessage time.Time

	VerboseCallbackHandler bool
	Log                    *log.Logger

	stopped bool
	quit    bool
}

// A struct to represent an event.
type Event struct {
	Code       string
	Raw        string
	Nick       string //<nick>
	Host       string //<nick>!<usr>@<host>
	Source     string //<host>
	User       string //<usr>
	Arguments  []string
	Connection *Connection
}

// Retrieve the last message from Event arguments.
// This function  leaves the arguments untouched and
// returns an empty string if there are none.
func (e *Event) Message() string {
	if len(e.Arguments) == 0 {
		return ""
	}
	return e.Arguments[len(e.Arguments)-1]
}
