package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var (
	// NO is "no" string bytes
	NO = []byte("NO")
	// HI is default "hi"
	HI = []byte("Hi~")
)

// Body 是消息正文
type Body struct {
	userID  string
	message []byte
}

// Group 是一个用户组，组内用户消息广播
type Group struct {
	// Group ID
	id int
	// User in the group
	users []string
	// 当前用户组的持续时间，一旦超过，用户组解散
	duration int
	// 当前用户组的共同喜好
	likes []string
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Register clients
	clients map[string]*Client

	// Inbound messages from the clients.
	broadcast chan *Body

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Idle clients
	idles map[string]*Client

	// All waiting users
	waitings map[string]*User

	// Friendship of offlines users, key is offline user, value is the user's friend
	offlines map[string]*User

	// All invited users, key is inviter, value is invitee
	friends map[string]*User

	// All valid group
	groups map[int]*Group

	// All likes and dislikes
	likes []string
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan *Body),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[string]*Client),
		idles:      make(map[string]*Client),
		waitings:   make(map[string]*User),
		offlines:   make(map[string]*User),
		friends:    make(map[string]*User),
		groups:     make(map[int]*Group),
		likes:      make([]string, 0),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client.user.ID] = client
			if friend, ok := h.offlines[client.user.ID]; ok {
				h.friends[client.user.ID] = friend
				if friendClient, ok := h.clients[friend.ID]; ok {
					sendMessage(h, friendClient, []byte("对方已上线！"))
				}
				delete(h.offlines, client.user.ID)
			} else {
				sendMessage(h, client, []byte(`正在为你寻找旅友～`))
				go h.find(client)
			}
			fmt.Println("register:", len(h.clients))
		case client := <-h.unregister:
			if _, ok := h.clients[client.user.ID]; ok {
				delete(h.clients, client.user.ID)
				delete(h.idles, client.user.ID)
				close(client.send)
				fmt.Println("unregister:", len(h.clients))
				friend, ok := h.friends[client.user.ID]
				if !ok { // 当前用户没有朋友关系
					continue
				}

				ff, ok := h.friends[friend.ID]
				if ok && ff.ID == client.user.ID {
					h.offlines[client.user.ID] = friend
					if friendClient, ok := h.clients[friend.ID]; ok {
						sendMessage(h, friendClient, []byte("对方已离线~"))
					}
				} else {
					delete(h.friends, client.user.ID)
					delete(h.friends, friend.ID)
				}
			}
		case body := <-h.broadcast:
			me, ok := h.clients[body.userID]
			friend, ok1 := h.friends[me.user.ID]
			if ok && ok1 {
				// 如果用户的朋友不是空闲状态，
				// 则表示该用户已经成为其他用户的朋友
				// 绿皮车的机制是，用户始终只能和一位朋友聊天

				if friendClient, ok := h.idles[friend.ID]; ok {
					if equale(NO, body.message) {
						delete(h.friends, me.user.ID)
					} else {
						h.friends[friend.ID] = me.user
						delete(h.idles, me.user.ID)
						delete(h.idles, friendClient.user.ID)

						sendMessage(h, friendClient, []byte("找到啦～"))
						sendMessage(h, friendClient, body.message)

						likes, _ := friendClient.user.likes(me.user)
						group := &Group{
							id:       len(h.groups),
							users:    []string{me.user.ID, friendClient.user.ID},
							duration: rand.Intn(30) + 5,
						}
						group.likes = likes
						h.groups[group.id] = group

						go h.chat(group)
					}
				} else {
					if ff, ok := h.friends[friend.ID]; ok && ff.ID == me.user.ID {
						// 如果朋友的朋友是自己，那么就可以给对方发消息啦~

						friendClient, ok = h.clients[friend.ID]
						if ok {
							sendMessage(h, friendClient, body.message)
						} else {
							sendMessage(h, me, []byte("对方还未上线哦~"))
						}
					} else {
						delete(h.friends, me.user.ID)
						h.idles[body.userID] = me
						sendMessage(h, me, []byte("对方已找到新旅友~，正在为你寻找新旅友~"))
						go h.find(me)
					}
				}
			} else {
				sendMessage(h, me, []byte("还没有旅友哦~"))
			}
		}
	}
}

// find friend
func (h *Hub) find(c *Client) {
	h.idles[c.user.ID] = c

	count := len(h.idles) - 1
	if 0 == count { // 表示当前只有一个用户（当前用户）处于空闲中
		return
	}

	keys1 := make([]string, 0, count)
	keys2 := make([]string, 0, count)
	keys3 := make([]string, 0, count)
	for k, v := range h.idles {
		if k == c.user.ID {
			continue
		}
		if _, ok := h.friends[v.user.ID]; ok {
			continue
		}
		if 0 == len(v.user.Likes) { // 如果对方没有喜欢的，则进第三列表
			keys3 = append(keys3, k)
			continue
		}

		_, likeGroupCount := c.user.likes(v.user)
		_, dislikeGroupCount := c.user.disLikes(v.user)

		// 讨厌的事物比共同喜好的事物要多时，则不考虑该旅友
		if likeGroupCount < dislikeGroupCount {
			continue
		}

		// 有五个共同爱好的，进第一优先列表
		// 以下同理

		if 5 <= likeGroupCount {
			keys1 = append(keys1, k)
		} else if 3 <= likeGroupCount {
			keys2 = append(keys2, k)
		} else {
			keys3 = append(keys3, k)
		}
	}

	v, ok := h.getOne(keys1)
	if !ok { // 当第一优先列表没有符合要求者，考虑第二列表，以下同理
		v, ok = h.getOne(keys2)
	}
	if !ok {
		v, ok = h.getOne(keys3)
	}

	if ok {
		h.friends[v.user.ID] = c.user
		if c.user.Hi == "" {
			sendMessage(h, v, HI)
		} else {
			sendMessage(h, v, []byte(c.user.Hi))
		}
		sendMessage(h, v, []byte(`回复 NO 拒绝，回复其他开始旅行`))
	}
}

func (h *Hub) chat(g *Group) {
	ticker := time.NewTicker(time.Duration(g.duration) * time.Minute)
	defer func() {
		ticker.Stop()
		delete(h.groups, g.id)
	}()
	for _, userID := range g.users {
		if client, ok := h.clients[userID]; ok {
			sendMessage(h, client, []byte(fmt.Sprintf("开始旅行，本次旅行时间是 %v 分钟", g.duration)))
			if 0 != len(g.likes) {
				sendMessage(h, client, []byte("你们之间共同的喜好是："))
				sendMessage(h, client, []byte(strings.Join(g.likes, ",")))
			}
		}
	}

	for {
		select {
		case <-ticker.C:
			for _, userID := range g.users {
				delete(h.friends, userID)
				delete(h.offlines, userID)
				if client, ok := h.clients[userID]; ok {
					h.idles[userID] = client
					sendMessage(h, client, []byte("已到达目的地，本次旅行结束~"))
				}
			}
			return
		}
	}
}

func (h *Hub) getOne(elems []string) (*Client, bool) {
	count := len(elems)
	if 0 == count {
		return nil, false
	}
	if 1 == count {
		v, ok := h.idles[elems[0]]
		return v, ok
	}

	i := rand.Intn(count)
	v, ok := h.idles[elems[i]]
	return v, ok
}

func (h *Hub) deduplication() {
	likes := make(map[string]bool)
	pure := make([]string, 0, len(h.likes))
	for _, v := range h.likes {
		if _, ok := likes[v]; !ok {
			pure = append(pure, v)
			likes[v] = true
		}
	}
	h.likes = pure
}

func sendMessage(h *Hub, c *Client, msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		close(c.send)
		delete(h.clients, c.user.ID)
		delete(h.idles, c.user.ID)
		return false
	}
}

func equale(bs1, bs2 []byte) bool {
	if len(bs1) != len(bs2) {
		return false
	}

	for i, b := range bs1 {
		if b != bs2[i] {
			return false
		}
	}

	return true
}

// likeRegexp 将喜好转换为正则表达式，方便与其他用户匹配
func likeRegexp(elems []string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0]
	}

	result := ""
	for _, elem := range elems {
		// (apple)? 可以匹配一个或零个的相同字符串，
		// 当匹配结果为一个时，表示有共同喜好（例子中为 apple），
		// 为零个时，表示该爱好对方没有
		result += fmt.Sprintf("(%v)?", elem)
	}

	return result
}
