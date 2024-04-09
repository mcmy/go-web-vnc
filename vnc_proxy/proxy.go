package vnc_proxy

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type TokenHandler func(r *http.Request) (addr string, err error)

// Config represents vnc proxy config
type Config struct {
	DialTimeout time.Duration
	TokenHandler
}

// Proxy represents vnc proxy
type Proxy struct {
	dialTimeout  time.Duration // Timeout for connecting to each target vnc server
	peers        map[*peer]struct{}
	l            sync.RWMutex
	tokenHandler TokenHandler
}

// New returns a vnc proxy
// If token handler is nil, vnc backend address will always be :5901
func New(conf *Config) *Proxy {
	if conf.TokenHandler == nil {
		conf.TokenHandler = func(r *http.Request) (addr string, err error) {
			return ":5901", nil
		}
	}

	return &Proxy{
		dialTimeout:  conf.DialTimeout,
		peers:        make(map[*peer]struct{}),
		l:            sync.RWMutex{},
		tokenHandler: conf.TokenHandler,
	}
}

// ServeWS provides websocket handler
func (p *Proxy) ServeWS(ws *websocket.Conn) {
	log.Println("ServeWS")
	ws.PayloadType = websocket.BinaryFrame

	r := ws.Request()
	log.Printf("request url: %v\n", r.URL)

	// get vnc backend server addr
	addr, err := p.tokenHandler(r)
	if err != nil {
		log.Printf("get vnc backend failed: %v\n", err)
		return
	}

	peer, err := NewPeer(ws, addr, p.dialTimeout)
	if err != nil {
		log.Printf("new vnc peer failed: %v\n", err)
		return
	}

	p.addPeer(peer)
	defer func() {
		log.Println("close peer")
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
	p.l.RLock()
	defer p.l.RUnlock()
	return p.peers
}
