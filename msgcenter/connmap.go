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
	"errors"
	"github.com/petar/GoLLRB/llrb"
)

type minimalConn interface {
	Username() string
	UniqId() string
}

type connMap interface {
	AddConn(conn minimalConn, maxNrConnsPerUser int, maxNrUsers int) error
	GetConn(username string) []minimalConn
	DelConn(conn minimalConn) bool
}

type connListItem struct {
	name string
	list []minimalConn
}

func (self *connListItem) key() string {
	if len(self.list) == 0 {
		return self.name
	}
	return connKey(self.list[0])
}

func (self *connListItem) Less(than llrb.Item) bool {
	selfKey := llrb.String(self.key())
	thanKey := llrb.String(than.(*connListItem).key())
	return selfKey.Less(thanKey)
}

func connKey(conn minimalConn) string {
	return conn.Username()
}

/*
func getKey(a interface{}) string {
	switch t := a.(type) {
	case string:
		return t
	case []minimalConn:
		if len(t) > 0 {
			return connKey(t[0])
		}
	}
	return ""
}

func lessConnList(a, b interface{}) bool {
	akey := getKey(a)
	bkey := getKey(b)
	cmp := bytes.Compare([]byte(akey), []byte(bkey))
	return cmp < 0
}
*/

type treeBasedConnMap struct {
	tree *llrb.LLRB
}

func (self *treeBasedConnMap) GetConn(user string) []minimalConn {
	key := &connListItem{name: user, list: nil}
	clif := self.tree.Get(key)
	cl, ok := clif.(*connListItem)
	if !ok || cl == nil {
		return nil
	}
	return cl.list
}

var ErrTooManyUsers = errors.New("too many users")
var ErrTooManyConnForThisUser = errors.New("too many connections under this user")

func (self *treeBasedConnMap) AddConn(conn minimalConn, maxNrConnsPerUser int, maxNrUsers int) error {
	if conn == nil {
		return nil
	}
	var cl []minimalConn
	cl = self.GetConn(connKey(conn))
	if cl == nil {
		if maxNrUsers > 0 && self.tree.Len() >= maxNrUsers {
			return ErrTooManyUsers
		}
		cl = make([]minimalConn, 0, 3)
	}
	if maxNrConnsPerUser > 0 && len(cl) >= maxNrConnsPerUser {
		return ErrTooManyConnForThisUser
	}
	for _, c := range cl {
		if c.UniqId() == conn.UniqId() {
			return nil
		}
	}
	cl = append(cl, conn)
	key := &connListItem{name: "", list: cl}
	self.tree.ReplaceOrInsert(key)
	return nil
}

func (self *treeBasedConnMap) DelConn(conn minimalConn) bool {
	if conn == nil {
		return false
	}
	cl := self.GetConn(connKey(conn))
	if cl == nil {
		return false
	}
	i := -1
	var c minimalConn
	for i, c = range cl {
		if c.UniqId() == conn.UniqId() {
			break
		}
	}
	if i < 0 {
		return false
	}
	if len(cl) == 1 {
		key := &connListItem{name: connKey(conn), list: cl}
		c := self.tree.Delete(key)
		if c == nil {
			return false
		}
		return true
	}
	cl[i] = cl[len(cl)-1]
	cl = cl[:len(cl)-1]
	key := &connListItem{name: connKey(conn), list: cl}
	if len(cl) == 0 {
		self.tree.Delete(key)
	} else {
		self.tree.ReplaceOrInsert(key)
	}
	return true
}

func newTreeBasedConnMap() connMap {
	ret := new(treeBasedConnMap)
	ret.tree = llrb.New()
	return ret
}
