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

package msgcenter

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/uniqush/uniqush-conn/evthandler"
	"github.com/uniqush/uniqush-conn/msgcache"
	"github.com/uniqush/uniqush-conn/proto"
	"github.com/uniqush/uniqush-conn/proto/server"
	"github.com/uniqush/uniqush-conn/push"
	"strings"
	"sync"
	"time"
)

type eventConnIn struct {
	errChan chan error
	conn    server.Conn
}

type eventConnLeave struct {
	conn server.Conn
	err  error
}

type Result struct {
	Err     error  `json:"err,omitempty"`
	ConnId  string `json:"connId,omitempty"`
	Visible bool   `json:"visible"`
}

func (self *Result) Error() string {
	b, _ := json.Marshal(self)
	return string(b)
}

type ServiceConfig struct {
	MaxNrConns        int
	MaxNrUsers        int
	MaxNrConnsPerUser int

	MsgCache msgcache.Cache

	LoginHandler          evthandler.LoginHandler
	LogoutHandler         evthandler.LogoutHandler
	MessageHandler        evthandler.MessageHandler
	ForwardRequestHandler evthandler.ForwardRequestHandler
	ErrorHandler          evthandler.ErrorHandler

	// Push related web hooks
	SubscribeHandler   evthandler.SubscribeHandler
	UnsubscribeHandler evthandler.UnsubscribeHandler
	PushHandler        evthandler.PushHandler

	PushService push.Push
}

type writeMessageRequest struct {
	user    string
	msg     *proto.Message
	ttl     time.Duration
	extra   map[string]string
	resChan chan<- []*Result
}

type serviceCenter struct {
	serviceName string
	config      *ServiceConfig
	fwdChan     chan<- *server.ForwardRequest

	writeReqChan chan *writeMessageRequest
	connIn       chan *eventConnIn
	connLeave    chan *eventConnLeave
	subReqChan   chan *server.SubscribeRequest

	pushServiceLock sync.RWMutex
}

var ErrTooManyConns = errors.New("too many connections")
var ErrInvalidConnType = errors.New("invalid connection type")

func (self *serviceCenter) ReceiveForward(fwdreq *server.ForwardRequest) {
	shouldFwd := false
	if self.config != nil {
		if self.config.ForwardRequestHandler != nil {
			shouldFwd = self.config.ForwardRequestHandler.ShouldForward(fwdreq)
			maxttl := self.config.ForwardRequestHandler.MaxTTL()
			if fwdreq.TTL < 1*time.Second || fwdreq.TTL > maxttl {
				fwdreq.TTL = maxttl
			}
		}
	}
	if !shouldFwd {
		return
	}
	receiver := fwdreq.Receiver
	extra := getPushInfo(fwdreq.Message, nil, true)
	self.SendMessage(receiver, fwdreq.Message, extra, fwdreq.TTL)
}

func getPushInfo(msg *proto.Message, extra map[string]string, fwd bool) map[string]string {
	if extra == nil {
		extra = make(map[string]string, len(msg.Header)+3)
	}
	if fwd {
		for k, v := range msg.Header {
			if strings.HasPrefix(k, "notif.") {
				if strings.HasPrefix(k, "notif.uniqush.") {
					// forward message should not contain reserved fields
					continue
				}
				extra[k] = v
				delete(msg.Header, k)
			}
		}
		extra["uniqush.sender"] = msg.Sender
		extra["uniqush.sender-service"] = msg.SenderService
	}
	if msg.Header != nil {
		if title, ok := msg.Header["title"]; ok {
			if _, ok = extra["notif.msg"]; !ok {
				extra["notif.msg"] = title
			}
		}
	}
	extra["notif.uniqush.msgsize"] = fmt.Sprintf("%v", msg.Size())
	return extra
}

func (self *serviceCenter) shouldPush(service, username string, msg *proto.Message, extra map[string]string, fwd bool) bool {
	if self.config != nil {
		if self.config.PushHandler != nil {
			info := getPushInfo(msg, extra, fwd)
			return self.config.PushHandler.ShouldPush(service, username, info)
		}
	}
	return false
}

func (self *serviceCenter) subscribe(req *server.SubscribeRequest) {
	if req == nil {
		return
	}
	if self.config != nil {
		if self.config.PushService != nil {
			if req.Subscribe {
				self.config.PushService.Subscribe(req.Service, req.Username, req.Params)
			} else {
				self.config.PushService.Unsubscribe(req.Service, req.Username, req.Params)
			}
		}
	}
}

func (self *serviceCenter) nrDeliveryPoints(service, username string) int {
	n := 0
	if self.config != nil {
		if self.config.PushService != nil {
			n = self.config.PushService.NrDeliveryPoints(service, username)
		}
	}
	return n
}

func (self *serviceCenter) pushNotif(service, username string, msg *proto.Message, extra map[string]string, msgIds []string, fwd bool) {
	if self.config != nil {
		if self.config.PushService != nil {
			info := getPushInfo(msg, extra, fwd)
			err := self.config.PushService.Push(service, username, info, msgIds)
			if err != nil {
				self.reportError(service, username, "", "", err)
			}
		}
	}
}

func (self *serviceCenter) reportError(service, username, connId, addr string, err error) {
	if self.config != nil {
		if self.config.ErrorHandler != nil {
			go self.config.ErrorHandler.OnError(service, username, connId, addr, err)
		}
	}
}

func (self *serviceCenter) reportLogin(service, username, connId, addr string) {
	if self.config != nil {
		if self.config.LoginHandler != nil {
			go self.config.LoginHandler.OnLogin(service, username, connId, addr)
		}
	}
}

func (self *serviceCenter) reportMessage(connId string, msg *proto.Message) {
	if self.config != nil {
		if self.config.MessageHandler != nil {
			go self.config.MessageHandler.OnMessage(connId, msg)
		}
	}
}

func (self *serviceCenter) reportLogout(service, username, connId, addr string, err error) {
	if self.config != nil {
		if self.config.LogoutHandler != nil {
			go self.config.LogoutHandler.OnLogout(service, username, connId, addr, err)
		}
	}
}

func (self *serviceCenter) cacheMessage(service, username string, msg *proto.Message, ttl time.Duration) (id string, err error) {
	if self.config != nil {
		if self.config.MsgCache != nil {
			id, err = self.config.MsgCache.CacheMessage(service, username, msg, ttl)
		}
	}
	return
}

type connWriteErr struct {
	conn server.Conn
	err  error
}

func (self *serviceCenter) process(maxNrConns, maxNrConnsPerUser, maxNrUsers int) {
	connMap := newTreeBasedConnMap()
	nrConns := 0
	for {
		select {
		case connInEvt := <-self.connIn:
			if maxNrConns > 0 && nrConns >= maxNrConns {
				if connInEvt.errChan != nil {
					connInEvt.errChan <- ErrTooManyConns
				}
				continue
			}
			err := connMap.AddConn(connInEvt.conn, maxNrConnsPerUser, maxNrUsers)
			if err != nil {
				if connInEvt.errChan != nil {
					connInEvt.errChan <- err
				}
				continue
			}
			nrConns++
			if connInEvt.errChan != nil {
				connInEvt.errChan <- nil
			}
		case leaveEvt := <-self.connLeave:
			deleted := connMap.DelConn(leaveEvt.conn)
			fmt.Printf("delete a connection %v under user %v; deleted: %v\n", leaveEvt.conn.UniqId(), leaveEvt.conn.Username(), deleted)
			leaveEvt.conn.Close()
			if deleted {
				nrConns--
				conn := leaveEvt.conn
				self.reportLogout(conn.Service(), conn.Username(), conn.UniqId(), conn.RemoteAddr().String(), leaveEvt.err)
			}
		case subreq := <-self.subReqChan:
			self.pushServiceLock.Lock()
			self.subscribe(subreq)
			self.pushServiceLock.Unlock()
		case wreq := <-self.writeReqChan:
			conns := connMap.GetConn(wreq.user)
			res := make([]*Result, 0, len(conns))
			errConns := make([]*connWriteErr, 0, len(conns))
			n := 0
			for _, conn := range conns {
				if conn == nil {
					continue
				}
				var err error
				sconn, ok := conn.(server.Conn)
				if !ok {
					continue
				}
				_, err = sconn.SendMessage(wreq.msg, wreq.extra, wreq.ttl)
				if err != nil {
					errConns = append(errConns, &connWriteErr{sconn, err})
					res = append(res, &Result{err, sconn.UniqId(), sconn.Visible()})
					self.reportError(sconn.Service(), sconn.Username(), sconn.UniqId(), sconn.RemoteAddr().String(), err)
					continue
				} else {
					res = append(res, &Result{nil, sconn.UniqId(), sconn.Visible()})
				}
				if sconn.Visible() {
					n++
				}
			}

			if n == 0 {
				msg := wreq.msg
				extra := wreq.extra
				username := wreq.user
				service := self.serviceName
				fwd := false
				if len(msg.Sender) > 0 && len(msg.SenderService) > 0 {
					if msg.Sender != username || msg.SenderService != service {
						fwd = true
					}
				}
				go func() {
					should := self.shouldPush(service, username, msg, extra, fwd)
					if !should {
						return
					}
					self.pushServiceLock.RLock()
					defer self.pushServiceLock.RUnlock()
					n := self.nrDeliveryPoints(service, username)
					if n <= 0 {
						return
					}
					var msgIds []string
					msgIds = make([]string, n)
					var e error
					for i := 0; i < n; i++ {
						msgIds[i], e = self.cacheMessage(service, username, msg, wreq.ttl)
						if e != nil {
							// FIXME: Dark side of the force
							return
						}
					}
					self.pushNotif(service, username, msg, extra, msgIds, fwd)
				}()
			}
			if wreq.resChan != nil {
				wreq.resChan <- res
			}

			// close all connections with error:
			go func() {
				for _, e := range errConns {
					fmt.Printf("Need to remove connection %v\n", e.conn.UniqId())
					self.connLeave <- &eventConnLeave{conn: e.conn, err: e.err}
				}
			}()
		}
	}
}

func (self *serviceCenter) SendMessage(username string, msg *proto.Message, extra map[string]string, ttl time.Duration) []*Result {
	req := new(writeMessageRequest)
	ch := make(chan []*Result)
	req.msg = msg
	req.user = username
	req.ttl = ttl
	req.resChan = ch
	req.extra = extra
	self.writeReqChan <- req
	res := <-ch
	return res
}

func (self *serviceCenter) serveConn(conn server.Conn) {
	conn.SetForwardRequestChannel(self.fwdChan)
	conn.SetSubscribeRequestChan(self.subReqChan)
	var err error
	defer func() {
		self.connLeave <- &eventConnLeave{conn: conn, err: err}
	}()
	for {
		var msg *proto.Message
		msg, err = conn.ReadMessage()
		if err != nil {
			return
		}
		self.reportMessage(conn.UniqId(), msg)
	}
}

func (self *serviceCenter) NewConn(conn server.Conn) error {
	usr := conn.Username()
	if len(usr) == 0 || strings.Contains(usr, ":") || strings.Contains(usr, "\n") {
		return fmt.Errorf("[Username=%v] Invalid Username")
	}
	evt := new(eventConnIn)
	ch := make(chan error)

	conn.SetMessageCache(self.config.MsgCache)
	evt.conn = conn
	evt.errChan = ch
	self.connIn <- evt
	err := <-ch
	if err == nil {
		go self.serveConn(conn)
		self.reportLogin(conn.Service(), usr, conn.UniqId(), conn.RemoteAddr().String())
	}
	return err
}

func newServiceCenter(serviceName string, conf *ServiceConfig, fwdChan chan<- *server.ForwardRequest) *serviceCenter {
	ret := new(serviceCenter)
	ret.config = conf
	if ret.config == nil {
		ret.config = new(ServiceConfig)
	}
	ret.serviceName = serviceName
	ret.fwdChan = fwdChan

	ret.connIn = make(chan *eventConnIn)
	ret.connLeave = make(chan *eventConnLeave)
	ret.writeReqChan = make(chan *writeMessageRequest)
	ret.subReqChan = make(chan *server.SubscribeRequest)
	go ret.process(conf.MaxNrConns, conf.MaxNrConnsPerUser, conf.MaxNrUsers)
	return ret
}
