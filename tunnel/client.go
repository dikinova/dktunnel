package tunnel

import (
	"container/heap"
	"net"
	"sync"
	"time"
	mrand "math/rand"
	"io"
)

const (
	HeartBeartSpan = 5
)

type APP interface {
	Start() error
	Status(w io.Writer)
}

/// client hub
type ClientHub struct {
	*Hub
	hPriority int // current link count
	hIndex    int // index in the heap
}

func newClientHub(tunnel *Tunnel) *ClientHub {
	h := &ClientHub{
		Hub: newHub(tunnel),
	}
	h.Hub.onCtrlFilter = h.onCtrl
	go h.heartbeat()
	return h
}

func (h *ClientHub) heartbeat() {
	//心跳.用一个较小的周期偏差,将不同hub的心跳时间错开
	// 客户端发送ping,server端回传pong.
	tspan := time.Duration(1000*HeartBeartSpan + mrand.Intn(1000))
	ticker := time.NewTicker(time.Millisecond * tspan)
	defer ticker.Stop()
	for range ticker.C {
		if !h.SendCmd(0, CD_HEARTBEAT) {
			break
		}
	}
}

func (h *ClientHub) onCtrl(cmd Ctrl) bool {
	switch cmd.Code {
	case CD_HEARTBEAT:
		return true
	}
	return false
}

func (h *ClientHub) Status(w io.Writer) {
	h.Hub.Status(w)
	Info("priority:%d, index:%d", h.hPriority, h.hIndex)
}

type clientHubQueue []*ClientHub

func (cq clientHubQueue) Len() int {
	return len(cq)
}

func (cq clientHubQueue) Less(i, j int) bool {
	return cq[i].hPriority < cq[j].hPriority
}

func (cq clientHubQueue) Swap(i, j int) {
	cq[i], cq[j] = cq[j], cq[i]
	cq[i].hIndex = i
	cq[j].hIndex = j
}

func (cq *clientHubQueue) Push(x interface{}) {
	n := len(*cq)
	hub := x.(*ClientHub)
	hub.hIndex = n
	*cq = append(*cq, hub)
}

func (cq *clientHubQueue) Pop() interface{} {
	old := *cq
	n := len(old)
	hub := old[n-1]
	hub.hIndex = -1
	*cq = old[0 : n-1]
	return hub
}

/// tunnel client
type Client struct {
	laddr   string
	backend string
	secret  string
	tunnels uint

	hq   clientHubQueue
	lock sync.Mutex
}

func (cli *Client) createHub() (hub *ClientHub, err error) {
	conn, err := dial(cli.backend)
	if err != nil {
		return
	}
	Debug("client dial OK")

	tunnel := newTunnel(conn)
	helloA := newHelloA(cli.secret)

	if err = tunnel.WritePacket(0, helloA.toBytes(), true); err != nil {
		Error("write token failed(%v):%s", tunnel, err)
		return
	}

	_, helloB, err := tunnel.ReadPacket()
	if err != nil {
		Error("read challenge failed(%v):%s", tunnel, err)
		return
	}

	taa := NewTaa(cli.secret)
	helloC, err := taa.ExchangeCipherBlock(helloB)
	if err != nil {
		Error("exchange challenge failed(%v) %v", tunnel, err)
		return
	}

	if err = tunnel.WritePacket(0, helloC, true); err != nil {
		Error("write token failed(%v):%s", tunnel, err)
		return
	}

	tunnel.tconn.setKeys(taa.Token, cli.secret, true)

	hub = newClientHub(tunnel)
	hub.tunnel.tunId = taa.Token.ToID()

	Warn("client: %v, handshake succeed", hub.tunnel)
	return
}

func (cli *Client) addHub(item *ClientHub) {
	cli.lock.Lock()
	defer cli.lock.Unlock()
	heap.Push(&cli.hq, item)
}

func (cli *Client) removeHub(item *ClientHub) {
	cli.lock.Lock()
	defer cli.lock.Unlock()
	heap.Remove(&cli.hq, item.hIndex)
}

func (cli *Client) fetchHub() *ClientHub {
	cli.lock.Lock()
	defer cli.lock.Unlock()

	if len(cli.hq) == 0 {
		return nil
	}
	item := cli.hq[0]
	item.hPriority += 1
	heap.Fix(&cli.hq, 0)
	return item
}

func (cli *Client) downHub(chub *ClientHub) {
	cli.lock.Lock()
	defer cli.lock.Unlock()
	chub.hPriority -= 1
	heap.Fix(&cli.hq, chub.hIndex)
}

func (cli *Client) handleLinkConn(chub *ClientHub, kconn *net.TCPConn) {
	defer Recover()
	defer cli.downHub(chub)
	CT(T_Coroutine, OP_Increase)
	defer CT(T_Coroutine, OP_Decrease)

	id := LinkNextId()

	h := chub.Hub
	k := h.createLink(id)
	defer h.deleteLink(id)

	h.SendCmd(id, CD_LINK_CREATE)
	h.runLink(k, kconn)
}

func (cli *Client) listen() error {
	listener, err := net.Listen("tcp", cli.laddr)
	if err != nil {
		return err
	}

	defer listener.Close()

	tcpListener := listener.(*net.TCPListener)
	for {
		kconn, err := tcpListener.AcceptTCP()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				Warn("acceept failed temporary: %s", netErr.Error())
				continue
			} else {
				return err
			}
		}
		Info("new connection from %v", kconn.RemoteAddr())
		chub := cli.fetchHub()
		if chub == nil {
			Error("no active hub")
			kconn.Close()
			continue
		}

		// 这是link对应的连接的时间设置
		// 这不是TCP标准的一部分,并且不同的平台有不同的实现
		// 而且设置的时间生效比较慢.
		kconn.SetKeepAlive(true)
		kconn.SetKeepAlivePeriod(time.Second * 30)
		go cli.handleLinkConn(chub, kconn)
	}
}

func (cli *Client) Start() error {
	sz := cap(cli.hq)
	for i := 0; i < sz; i++ {
		go func(index int) {
			defer Recover()
			CT(T_Coroutine, OP_Increase)
			defer CT(T_Coroutine, OP_Decrease)

			for {
				hub, err := cli.createHub()
				if err != nil {
					Warn("client: %d tunnel, connect failed", index)
					time.Sleep(time.Second * (10))
					continue
				}

				cli.addHub(hub)
				hub.Start()
				cli.removeHub(hub)
				Warn("client: %d tunnel %5d, disconnected", index, hub.tunnel.tunId)
			}
		}(i)
	}

	time.Sleep(time.Second * 2) // 暂停一下.等待hub都创建完毕.
	return cli.listen()
}

func (cli *Client) Status(w io.Writer) {
	cli.lock.Lock()
	defer cli.lock.Unlock()
	for _, hub := range cli.hq {
		hub.Status(w)
	}
}

func NewClient(listen, backend, secret string, tunnels uint) (*Client, error) {
	client := &Client{
		laddr:   listen,
		backend: backend,
		secret:  secret,
		tunnels: tunnels,

		hq: make(clientHubQueue, tunnels)[0:0],
	}
	return client, nil
}
