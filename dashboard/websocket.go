/*
Copyright 2018 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"golang.org/x/net/websocket"
)

const chBufSize = 100

type wsClient struct {
	ws       *websocket.Conn
	ch       chan []*metric
	closed   chan bool
	debugLog debugging
}

func (c *wsClient) sendCh() chan<- []*metric {
	return (chan<- []*metric)(c.ch)
}

// to keep websocket connection open
func (c *wsClient) receiveListen(closing chan<- *wsClient) {
	for {
		var v interface{}
		if err := websocket.Message.Receive(c.ws, v); err != nil {
			log.Printf("Error receiving ws messages, closing connection: %v", err)
			closing <- c
			break
		}
	}
}

func newClient(ws *websocket.Conn) *wsClient {
	ch := make(chan []*metric, chBufSize)
	closed := make(chan bool)

	return &wsClient{ws: ws, ch: ch, closed: closed, debugLog: debugLog}
}

type wsStreamHandler struct {
	chAddClient chan *wsClient
	chDelClient chan *wsClient
	chMsgs      chan []*metric
}

func (p *wsStreamHandler) handler(ws *websocket.Conn) {
	if ws == nil {
		panic("ws connect is nil")
	}
	c := newClient(ws)
	p.chAddClient <- c
	c.receiveListen(p.chDelClient)
	defer ws.Close()
}

func (p *wsStreamHandler) watchClients() {
	for {
		select {
		// new client
		case c := <-p.chAddClient:
			debugLog.Printf("Added new client with IP %v", clientAddr(c.ws.Request()))
			clients = append(clients, c)
			debugLog.Printf("%v clients connected.", len(clients))

		// remove a client
		case c := <-p.chDelClient:
			debugLog.Printf("Client with IP %v left", clientAddr(c.ws.Request()))
			for i := range clients {
				if clients[i] == c {
					clients = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			debugLog.Printf("%v clients connected.", len(clients))

		// broadcast message for all clients
		case msg := <-p.chMsgs:
			for _, c := range clients {
				go func(c *wsClient, msg []*metric) {
					if err := websocket.JSON.Send(c.ws, msg); err != nil {
						log.Printf("couldn't send message to client %v", err)
					}
				}(c, msg)
			}
		}
	}
}

func clientAddr(req *http.Request) string {
	ip, port, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		log.Printf("userip: %q is not IP:port, err: %v", req.RemoteAddr, err)
		return "N/A"
	}

	realIP := net.ParseIP(ip)
	if realIP == nil {
		debugLog.Printf("userip: %q is not IP, err: %v", req.RemoteAddr, err)
		return "N/A"
	}

	return fmt.Sprintf("%v:%v", realIP, port)
}
