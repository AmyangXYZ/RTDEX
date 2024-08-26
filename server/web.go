package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type WebServer struct {
	lAddr      string
	wsUpgrader websocket.Upgrader
}

func NewWebServer(addr string) *WebServer {
	return &WebServer{
		lAddr: addr,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

func (web *WebServer) Start() error {
	http.Handle("/", http.FileServer(http.Dir("./dist")))
	http.HandleFunc("/ws/packets", web.handlePacketWs)
	http.HandleFunc("/ws/sessions", web.handleSessionWs)
	http.HandleFunc("/ws/cache", web.handleCachedDataWs)
	http.HandleFunc("/api/cache", web.handleCachedData)
	fmt.Println("[Server] Web service on", web.lAddr)

	return http.ListenAndServe(web.lAddr, nil)
}

func (web *WebServer) handlePacketWs(w http.ResponseWriter, r *http.Request) {
	conn, err := web.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("[Server] Websocket upgrade error:", err)
		return
	}

	defer func() {
		conn.Close()
	}()

	allPackets := pktSniffer.Get(0)
	if err := conn.WriteJSON(allPackets); err != nil {
		return
	}
	lastSent := len(allPackets)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		newPackets := pktSniffer.Get(lastSent)
		if len(newPackets) > 0 {
			if err := conn.WriteJSON(newPackets); err != nil {
				return
			}
			lastSent += len(newPackets)
		}
	}
}

func (web *WebServer) handleSessionWs(w http.ResponseWriter, r *http.Request) {
	conn, err := web.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("[Server] Websocket upgrade error:", err)
		return
	}
	defer func() {
		conn.Close()
	}()

	sessions := []*Session{}
	sessionMgr.Sessions.Range(func(_, v interface{}) bool {
		sessions = append(sessions, v.(*Session))
		return true
	})
	if err := conn.WriteJSON(sessions); err != nil {
		return
	}

	timer := time.NewTicker(1 * time.Second)
	for range timer.C {
		sessions := []*Session{}
		sessionMgr.Sessions.Range(func(_, v interface{}) bool {
			sessions = append(sessions, v.(*Session))
			return true
		})
		if err := conn.WriteJSON(sessions); err != nil {
			return
		}
	}
}

func (web *WebServer) handleCachedDataWs(w http.ResponseWriter, r *http.Request) {
	conn, err := web.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("[Server] Websocket upgrade error:", err)
		return
	}
	defer func() {
		conn.Close()
	}()

	data := cache.GetAll()
	if err := conn.WriteJSON(data); err != nil {
		return
	}
	timer := time.NewTicker(1000 * time.Millisecond)
	for range timer.C {
		data := cache.GetAll()
		if err := conn.WriteJSON(data); err != nil {
			return
		}
	}
}

func (web *WebServer) handleCachedData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing 'name' parameter", http.StatusBadRequest)
		return
	}

	var data []byte

	if cachedData := cache.Get(name); cachedData != nil {
		data = cachedData.data
	} else {
		http.Error(w, "Data not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", name))
	_, err := w.Write(data)
	if err != nil {
		http.Error(w, "Error writing response", http.StatusInternalServerError)
		return
	}
}
