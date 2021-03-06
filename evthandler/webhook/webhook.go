/*
 * Copyright 2013 Nan Deng
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package webhook

import (
	"bytes"
	"encoding/json"
	"github.com/uniqush/uniqush-conn/proto"
	"github.com/uniqush/uniqush-conn/proto/server"
	"net"
	"net/http"
	"time"
)

type WebHook interface {
	SetURL(url string)
	SetTimeout(timeout time.Duration)
	SetDefault(d int)
}

type webHook struct {
	URL     string
	Timeout time.Duration
	Default int
}

func (self *webHook) SetURL(url string) {
	self.URL = url
}

func (self *webHook) SetTimeout(timeout time.Duration) {
	self.Timeout = timeout
}

func (self *webHook) SetDefault(d int) {
	self.Default = d
}

func timeoutDialler(ns time.Duration) func(net, addr string) (c net.Conn, err error) {
	return func(netw, addr string) (net.Conn, error) {
		c, err := net.Dial(netw, addr)
		if err != nil {
			return nil, err
		}
		if ns.Seconds() > 0.0 {
			c.SetDeadline(time.Now().Add(ns))
		}
		return c, nil
	}
}

func (self *webHook) post(data interface{}) int {
	if len(self.URL) == 0 || self.URL == "none" {
		return self.Default
	}
	jdata, err := json.Marshal(data)
	if err != nil {
		return self.Default
	}
	c := http.Client{
		Transport: &http.Transport{
			Dial: timeoutDialler(self.Timeout),
		},
	}
	resp, err := c.Post(self.URL, "application/json", bytes.NewReader(jdata))
	if err != nil {
		return self.Default
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

type loginEvent struct {
	Service  string `json:"service"`
	Username string `json:"username"`
	ConnID   string `json:"connId"`
	Addr     string `json:"addr"`
}

type LoginHandler struct {
	webHook
}

func (self *LoginHandler) OnLogin(service, username, connId, addr string) {
	self.post(&loginEvent{service, username, connId, addr})
}

type logoutEvent struct {
	Service  string `json:"service"`
	Username string `json:"username"`
	ConnID   string `json:"connId"`
	Addr     string `json:"addr"`
	Reason   string `json:"reason"`
}

type LogoutHandler struct {
	webHook
}

func (self *LogoutHandler) OnLogout(service, username, connId, addr string, reason error) {
	self.post(&logoutEvent{service, username, connId, addr, reason.Error()})
}

type messageEvent struct {
	ConnID string         `json:"connId"`
	Msg    *proto.Message `json:"msg"`
}

type MessageHandler struct {
	webHook
}

func (self *MessageHandler) OnMessage(connId string, msg *proto.Message) {
	evt := new(messageEvent)
	evt.ConnID = connId
	evt.Msg = msg
	self.post(evt)
}

type errorEvent struct {
	Service  string `json:"service"`
	Username string `json:"username"`
	ConnID   string `json:"connId"`
	Addr     string `json:"addr"`
	Reason   string `json:"reason"`
}

type ErrorHandler struct {
	webHook
}

func (self *ErrorHandler) OnError(service, username, connId, addr string, reason error) {
	self.post(&errorEvent{service, username, connId, addr, reason.Error()})
}

type ForwardRequestHandler struct {
	webHook
	maxTTL time.Duration
}

func (self *ForwardRequestHandler) ShouldForward(fwd *server.ForwardRequest) bool {
	return self.post(fwd) == 200
}

func (self *ForwardRequestHandler) SetMaxTTL(ttl time.Duration) {
	self.maxTTL = ttl
}

func (self *ForwardRequestHandler) MaxTTL() time.Duration {
	return self.maxTTL
}

type authEvent struct {
	Service  string `json:"service"`
	Username string `json:"username"`
	Token    string `json:"token"`
	Addr     string `json:"addr"`
}

type AuthHandler struct {
	webHook
}

func (self *AuthHandler) Authenticate(srv, usr, token, addr string) (pass bool, err error) {
	evt := new(authEvent)
	evt.Service = srv
	evt.Username = usr
	evt.Token = token
	evt.Addr = addr
	pass = self.post(evt) == 200
	return
}

type pushRelatedEvent struct {
	Service  string            `json:"service"`
	Username string            `json:"username"`
	Info     map[string]string `json:"info"`
}

type SubscribeHandler struct {
	webHook
}

func (self *SubscribeHandler) ShouldSubscribe(service, username string, info map[string]string) bool {
	evt := new(pushRelatedEvent)
	evt.Service = service
	evt.Username = username
	evt.Info = info
	return self.post(evt) == 200
}

type PushHandler struct {
	webHook
}

func (self *PushHandler) ShouldPush(service, username string, info map[string]string) bool {
	evt := new(pushRelatedEvent)
	evt.Service = service
	evt.Username = username
	evt.Info = info
	return self.post(evt) == 200
}

type UnsubscribeHandler struct {
	webHook
}

func (self *UnsubscribeHandler) OnUnsubscribe(service, username string, info map[string]string) {
	evt := new(pushRelatedEvent)
	evt.Service = service
	evt.Username = username
	evt.Info = info
	self.post(evt)
	return
}
