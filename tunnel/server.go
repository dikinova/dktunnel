package tunnel

import (
	"net"
	"time"
	"sync"
	"io"
)

/// server hub
type ServerHub struct {
	*Hub
	backendAddr *net.TCPAddr
}

func newServerHub(tunnel *Tunnel, baddr *net.TCPAddr) *ServerHub {
	sh := &ServerHub{
		Hub:         newHub(tunnel),
		backendAddr: baddr,
	}
	sh.Hub.onCtrlFilter = sh.onCtrl
	return sh
}

func (h *ServerHub) handleServerLink(k *Link) {
	defer Recover()
	defer h.deleteLink(k.id)
	CT(T_Coroutine, OP_Increase)
	defer CT(T_Coroutine, OP_Decrease)

	conn, err := net.DialTCP("tcp", nil, h.backendAddr)
	if err != nil {
		Error("link(%d) connect to backend failed, err:%v", k.id, err)
		h.SendCmd(k.id, CD_LINK_CLOSE)
		return
	}

	h.runLink(k, conn)
}

func (h *ServerHub) onCtrl(cmd Ctrl) bool {
	id := cmd.LinkId
	switch cmd.Code {
	case CD_LINK_CREATE:
		l := h.createLink(id)
		if l != nil {
			go h.handleServerLink(l)
		} else {
			h.SendCmd(id, CD_LINK_CLOSE)
		}
		return true
	case CD_HEARTBEAT:
		h.SendCmd(0, CD_HEARTBEAT)
		return true
	}
	return false
}

/// tunnel server
type Server struct {
	listener net.Listener
	baddr    *net.TCPAddr
	secret   string
	hubs     map[*ServerHub]bool
	mux      sync.Mutex
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer Recover()
	CT(T_Coroutine, OP_Increase)
	defer CT(T_Coroutine, OP_Decrease)

	tunnel := newTunnel(conn)

	_, _, err := tunnel.ReadPacket()
	if err != nil {
		Error("read helloA failed(%v):%s", tunnel, err)
		return
	}

	// authenticate connection
	taa := NewTaa(s.secret)
	taa.GenRandomToken() //服务器生成随机token 每个hub的token都不一样

	hello := taa.GenCipherBlock(nil)
	if err := tunnel.WritePacket(0, hello, true); err != nil {
		Error("write challenge failed(%v):%s", tunnel, err)
		return
	}

	_, token, err := tunnel.ReadPacket()
	if err != nil {
		Error("read token failed(%v):%s", tunnel, err)
		return
	}

	if !taa.VerifyCipherBlock(token) {
		Error("verify token failed(%v)", tunnel)
		return
	}

	tunnel.tconn.setKeys(taa.Token, s.secret, false)
	sh := newServerHub(tunnel, s.baddr)
	sh.tunnel.tunId = taa.Token.ToID()
	s.mux.Lock()
	s.hubs[sh] = true // map is not thread safe
	s.mux.Unlock()
	Warn("server: %v, handshake succeed", sh.tunnel)

	defer delete(s.hubs, sh)

	sh.Start()
}

func (s *Server) Start() error {
	defer s.listener.Close()
	for {
		tcpL := s.listener.(*net.TCPListener)
		conn, err := tcpL.AcceptTCP()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				Warn("server: acceept failed temporary: %s", netErr.Error())
				continue
			} else {
				return err
			}
		}
		Warn("server: new connection from %v", conn.RemoteAddr())
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Second * 60)
		go s.handleConn(conn)
	}
}

func (s *Server) Status(w io.Writer) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for hub := range s.hubs {
		hub.Status(w)
	}
}

/// create a tunnel server
func NewServer(listen, backend, secret string) (*Server, error) {
	listener, err := newListener(listen)
	if err != nil {
		return nil, err
	}

	baddr, err := net.ResolveTCPAddr("tcp", backend)
	if err != nil {
		return nil, err
	}

	s := &Server{
		listener: listener,
		baddr:    baddr,
		secret:   secret,
		hubs:     make(map[*ServerHub]bool),
	}
	return s, nil
}
