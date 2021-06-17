package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/gobuffalo/packr/v2"
)

var addr = flag.String("addr", ":8080", "http service address")

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	box := packr.NewBox("./web")
	bs, _ := box.Find("home.html")
	w.Write(bs)
}

func userHandle(h *Hub, w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	bs, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var _user User
	err = json.Unmarshal(bs, &_user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, joined := h.clients[_user.ID]

	_user.Hi = strings.Trim(_user.Hi, "\n")
	_user.Hi = strings.Trim(_user.Hi, " ")
	if !joined {
		h.waitings[_user.ID] = &_user
	} else {
		h.clients[_user.ID].user = &_user
	}

	h.likes = append(h.likes, _user.Likes...)
	h.likes = append(h.likes, _user.Dislikes...)
	h.deduplication()

	http.Error(w, "success", http.StatusOK)
}

func likesHandle(h *Hub, w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	defer r.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.likes)
}

func main() {
	fmt.Println("v1.2.0")
	flag.Parse()
	hub := newHub()
	go hub.run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		userHandle(hub, w, r)
	})
	http.HandleFunc("/likes", func(w http.ResponseWriter, r *http.Request) {
		likesHandle(hub, w, r)
	})
	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
