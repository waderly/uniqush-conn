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

package configparser

import (
	"fmt"
	"github.com/kylelemons/go-gypsy/yaml"
	"github.com/uniqush/uniqush-conn/evthandler"
	"github.com/uniqush/uniqush-conn/evthandler/webhook"
	"github.com/uniqush/uniqush-conn/msgcache"
	"github.com/uniqush/uniqush-conn/msgcenter"
	"github.com/uniqush/uniqush-conn/proto/server"
	"github.com/uniqush/uniqush-conn/push"
	"net"
	"strconv"
	"time"
)

type Config struct {
	HandshakeTimeout time.Duration
	HttpAddr         string
	Auth             server.Authenticator
	ErrorHandler     evthandler.ErrorHandler
	filename         string
	srvConfig        map[string]*msgcenter.ServiceConfig
	defaultConfig    *msgcenter.ServiceConfig
}

func (self *Config) AllServices() []string {
	ret := make([]string, 0, len(self.srvConfig))
	for srv, _ := range self.srvConfig {
		ret = append(ret, srv)
	}
	return ret
}

func (self *Config) ReadConfig(srv string) *msgcenter.ServiceConfig {
	if ret, ok := self.srvConfig[srv]; ok {
		return ret
	}
	return self.defaultConfig
}

func parseInt(node yaml.Node) (n int, err error) {
	if scalar, ok := node.(yaml.Scalar); ok {
		str := string(scalar)
		n, err = strconv.Atoi(str)
	} else {
		err = fmt.Errorf("Not a scalar")
	}
	return
}

func parseString(node yaml.Node) (str string, err error) {
	if node == nil {
		str = ""
		return
	}
	if scalar, ok := node.(yaml.Scalar); ok {
		str = string(scalar)
	} else {
		err = fmt.Errorf("Not a scalar")
	}
	return
}

func parseDuration(node yaml.Node) (t time.Duration, err error) {
	if scalar, ok := node.(yaml.Scalar); ok {
		t, err = time.ParseDuration(string(scalar))
	} else {
		err = fmt.Errorf("timeout should be a scalar")
	}
	return
}

type webhookInfo struct {
	url          string
	timeout      time.Duration
	defaultValue string
}

func parseWebHook(node yaml.Node) (hook *webhookInfo, err error) {
	if kv, ok := node.(yaml.Map); ok {
		hook = new(webhookInfo)
		if url, ok := kv["url"]; ok {
			hook.url, err = parseString(url)
			if err != nil {
				err = fmt.Errorf("webhook's url should be a string")
				return
			}
		} else {
			err = fmt.Errorf("webhook should have url")
			return
		}
		if timeout, ok := kv["timeout"]; ok {
			hook.timeout, err = parseDuration(timeout)
			if err != nil {
				err = fmt.Errorf("timeout error: %v", err)
				return
			}
		}
		if defaultValue, ok := kv["default"]; ok {
			hook.defaultValue, err = parseString(defaultValue)
			if err != nil {
				err = fmt.Errorf("webhook's default value should be a string")
				return
			}
		}
	} else {
		err = fmt.Errorf("webhook should be a map")
	}
	return
}

func setWebHook(hd webhook.WebHook, node yaml.Node, timeout time.Duration) error {
	hook, err := parseWebHook(node)
	if err != nil {
		return err
	}
	if hook.timeout < 0*time.Second {
		hook.timeout = timeout
	}
	hd.SetTimeout(hook.timeout)
	hd.SetURL(hook.url)
	if hook.defaultValue == "allow" {
		hd.SetDefault(200)
	} else {
		hd.SetDefault(404)
	}
	return nil
}

func parseAuthHandler(node yaml.Node, timeout time.Duration) (h server.Authenticator, err error) {
	hd := new(webhook.AuthHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseMessageHandler(node yaml.Node, timeout time.Duration) (h evthandler.MessageHandler, err error) {
	hd := new(webhook.MessageHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseErrorHandler(node yaml.Node, timeout time.Duration) (h evthandler.ErrorHandler, err error) {
	hd := new(webhook.ErrorHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseForwardRequestHandler(node yaml.Node, timeout time.Duration) (h evthandler.ForwardRequestHandler, err error) {
	hd := new(webhook.ForwardRequestHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	if kv, ok := node.(yaml.Map); ok {
		if ttlnode, ok := kv["max-ttl"]; ok {
			ttl, e := parseDuration(ttlnode)
			if e != nil {
				err = fmt.Errorf("max-ttl: %v", e)
				return
			}
			hd.SetMaxTTL(ttl)
		} else {
			hd.SetMaxTTL(24 * time.Hour)
		}
	}
	h = hd
	return
}

func parseLogoutHandler(node yaml.Node, timeout time.Duration) (h evthandler.LogoutHandler, err error) {
	hd := new(webhook.LogoutHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseLoginHandler(node yaml.Node, timeout time.Duration) (h evthandler.LoginHandler, err error) {
	hd := new(webhook.LoginHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseSubscribeHandler(node yaml.Node, timeout time.Duration) (h evthandler.SubscribeHandler, err error) {
	hd := new(webhook.SubscribeHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseUnsubscribeHandler(node yaml.Node, timeout time.Duration) (h evthandler.UnsubscribeHandler, err error) {
	hd := new(webhook.UnsubscribeHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parsePushHandler(node yaml.Node, timeout time.Duration) (h evthandler.PushHandler, err error) {
	hd := new(webhook.PushHandler)
	err = setWebHook(hd, node, timeout)
	if err != nil {
		return
	}
	h = hd
	return
}

func parseUniqushPush(node yaml.Node, timeout time.Duration) (p push.Push, err error) {
	kv, ok := node.(yaml.Map)
	if !ok {
		err = fmt.Errorf("uniqush-push information should be a map")
		return
	}
	addrN, ok := kv["addr"]
	if !ok {
		err = fmt.Errorf("cannot find addr field")
		return
	}
	addr, err := parseString(addrN)
	if !ok {
		err = fmt.Errorf("address error: %v", err)
		return
	}
	_, err = net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		err = fmt.Errorf("bad addres: %v", err)
		return
	}
	if to, ok := kv["timeout"]; ok {
		timeout, err = parseDuration(to)
		if err != nil {
			err = fmt.Errorf("bad timeout: %v", err)
			return
		}
	}
	p = push.NewUniqushPushClient(addr, timeout)
	return
}

func parseCache(node yaml.Node) (cache msgcache.Cache, err error) {
	if fields, ok := node.(yaml.Map); ok {
		engine := "redis"
		addr := ""
		password := ""
		name := "0"

		for k, v := range fields {
			switch k {
			case "engine":
				engine, err = parseString(v)
			case "addr":
				addr, err = parseString(v)
			case "password":
				password, err = parseString(v)
			case "name":
				name, err = parseString(v)
			}
			if err != nil {
				err = fmt.Errorf("[field=%v] %v", k, err)
				return
			}
		}
		if engine != "redis" {
			err = fmt.Errorf("database %v is not supported", engine)
			return
		}
		db := 0
		db, err = strconv.Atoi(name)
		if err != nil || db < 0 {
			err = fmt.Errorf("invalid database name: %v", name)
			return
		}
		cache = msgcache.NewRedisMessageCache(addr, password, db)
	} else {
		err = fmt.Errorf("database info should be a map")
	}
	return
}

func parseService(service string, node yaml.Node, defaultConfig *msgcenter.ServiceConfig) (config *msgcenter.ServiceConfig, err error) {
	if node == nil {
		config = defaultConfig
		return
	}
	fields, ok := node.(yaml.Map)
	if !ok {
		err = fmt.Errorf("[service=%v] Service information should be a map", service)
		return
	}
	timeout := 3 * time.Second

	if t, ok := fields["timeout"]; ok {
		timeout, err = parseDuration(t)
		if err != nil {
			err = fmt.Errorf("[service=%v][field=timeout] %v", service, err)
			return
		}
	}

	config = new(msgcenter.ServiceConfig)

	if defaultConfig != nil {
		*config = *defaultConfig
	}

	for name, value := range fields {
		switch name {
		case "msg":
			config.MessageHandler, err = parseMessageHandler(value, timeout)
		case "logout":
			config.LogoutHandler, err = parseLogoutHandler(value, timeout)
		case "login":
			config.LoginHandler, err = parseLoginHandler(value, timeout)
		case "fwd":
			config.ForwardRequestHandler, err = parseForwardRequestHandler(value, timeout)
		case "push":
			config.PushHandler, err = parsePushHandler(value, timeout)
		case "subscribe":
			config.SubscribeHandler, err = parseSubscribeHandler(value, timeout)
		case "unsubscribe":
			config.UnsubscribeHandler, err = parseUnsubscribeHandler(value, timeout)
		case "uniqush-push":
			fallthrough
		case "uniqush_push":
			config.PushService, err = parseUniqushPush(value, timeout)
		case "max-conns":
			fallthrough
		case "max_conns":
			config.MaxNrConns, err = parseInt(value)
		case "max-online-users":
			fallthrough
		case "max_online_users":
			config.MaxNrUsers, err = parseInt(value)
		case "max-conns-per-user":
			fallthrough
		case "max_conns_per_user":
			config.MaxNrConnsPerUser, err = parseInt(value)
		case "db":
			config.MsgCache, err = parseCache(value)
		case "err":
			config.ErrorHandler, err = parseErrorHandler(value, timeout)
		}
		if err != nil {
			err = fmt.Errorf("[service=%v][field=%v] %v", service, name, err)
			config = nil
			return
		}
	}
	return
}

func checkConfig(config *Config) error {
	if config.Auth == nil {
		return fmt.Errorf("No authentication url")
	}
	return nil
}

func Parse(filename string) (config *Config, err error) {
	file, err := yaml.ReadFile(filename)
	if err != nil {
		return
	}
	root := file.Root
	config = new(Config)
	config.filename = filename
	switch t := root.(type) {
	case yaml.Map:
		config.srvConfig = make(map[string]*msgcenter.ServiceConfig, len(t))
		if dc, ok := t["default"]; ok {
			config.defaultConfig, err = parseService("default", dc, nil)
		}
		if err != nil {
			config = nil
			return
		}
		for srv, node := range t {
			switch srv {
			case "auth":
				config.Auth, err = parseAuthHandler(node, 3*time.Second)
				if err != nil {
					err = fmt.Errorf("auth: %v", err)
					return
				}
				continue
			case "err":
				config.ErrorHandler, err = parseErrorHandler(node, 3*time.Second)
				if err != nil {
					err = fmt.Errorf("global error handler: %v", err)
					return
				}
				continue
			case "http-addr":
				fallthrough
			case "http_addr":
				config.HttpAddr, err = parseString(node)
				if err != nil {
					err = fmt.Errorf("Bad HTTP bind address: %v", err)
					return
				}
				continue
			case "handshake-timeout":
				fallthrough
			case "handshake_timeout":
				config.HandshakeTimeout, err = parseDuration(node)
				if err != nil {
					err = fmt.Errorf("bad handshake timeout: %v", err)
					return
				}
				continue
			case "default":
				// Don't need to parse the default service again.
				continue
			}
			var sconf *msgcenter.ServiceConfig
			sconf, err = parseService(srv, node, config.defaultConfig)
			if err != nil {
				config = nil
				return
			}
			config.srvConfig[srv] = sconf
		}
	default:
		err = fmt.Errorf("Top level should be a map")
	}
	if err == nil {
		err = checkConfig(config)
	}
	return
}
