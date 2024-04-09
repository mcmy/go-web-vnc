package proxy

import (
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"strings"
	"sync"
)

type TokenHandler func(r *http.Request) (addr string, err error)

type Config struct {
	TokenHandler
}

type Proxy struct {
	peers        map[*peer]struct{}
	l            sync.RWMutex
	tokenHandler TokenHandler
}

func New(conf *Config) *Proxy {
	if conf.TokenHandler == nil {
		conf.TokenHandler = func(r *http.Request) (addr string, err error) {
			return ":5901", nil
		}
	}

	return &Proxy{
		peers:        make(map[*peer]struct{}),
		l:            sync.RWMutex{},
		tokenHandler: conf.TokenHandler,
	}
}

func checkToken(token string) bool {
	return true
}

func (p *Proxy) ServeWS(ws *websocket.Conn) {
	ws.PayloadType = websocket.BinaryFrame

	r := ws.Request()

	// get vnc backend server addr
	addr, err := p.tokenHandler(r)
	if err != nil {
		log.Printf("get vnc backend failed: %v\n", err)
		return
	}

	peer, err := NewPeer(ws, addr)
	if err != nil {
		log.Printf("new vnc peer failed: %v\n", err)
		return
	}

	p.addPeer(peer)
	defer func() {
		log.Printf("close peer\n")
		p.deletePeer(peer)

	}()

	go func() {
		if err := peer.ReadTarget(); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Println(err)
			return
		}
	}()

	if err = peer.ReadSource(); err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			return
		}
		log.Println(err)
		return
	}
}

func (p *Proxy) addPeer(peer *peer) {
	p.l.Lock()
	p.peers[peer] = struct{}{}
	p.l.Unlock()
}

func (p *Proxy) deletePeer(peer *peer) {
	p.l.Lock()
	delete(p.peers, peer)
	peer.Close()
	p.l.Unlock()
}

func (p *Proxy) Peers() map[*peer]struct{} {
	return p.peers
}
