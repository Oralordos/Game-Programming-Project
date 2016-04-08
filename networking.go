package main

import (
	"encoding/json"
	"log"
	"net"

	"github.com/Oralordos/Game-Programming-Project/events"
	"github.com/nu7hatch/gouuid"
)

const port = "10328"

type Network struct {
	conn    net.Conn
	decoder *json.Decoder
	dir     int
	eventCh chan events.Event
	close   chan struct{}
}

func (n *Network) readloop() {
	for {
		var typ int
		err := n.decoder.Decode(&typ)
		if err != nil {
			if n, ok := err.(net.Error); ok {
				if n.Temporary() {
					continue
				}
			}
			log.Println(err)
			break
		}
		ev, err := events.DecodeJSON(typ, n.decoder)
		if err != nil {
			if n, ok := err.(net.Error); ok {
				if n.Temporary() {
					continue
				}
			}
			log.Println(err)
			break
		}
		ev.SetDuplicate(true)
		events.SendEvent(ev)
	}
}

func (n *Network) mainloop() {
loop:
	for {
		select {
		case ev := <-n.eventCh:
			n.handleEvent(ev)
		case _, ok := <-n.close:
			if !ok {
				break loop
			}
		}
	}
	events.RemoveListener(n.eventCh, n.dir, 0)
	n.conn.Close()
}

func (n *Network) Destroy() {
	close(n.close)
}

func (n *Network) handleEvent(ev events.Event) {
	if ev.HasDuplicate() {
		return
	}
	n.sendEvent(ev)
}

func (n *Network) sendEvent(ev events.Event) {
	encoder := json.NewEncoder(n.conn)
	err := encoder.Encode(ev.GetTypeID())
	if err != nil {
		if netErr, ok := err.(net.Error); ok {
			if netErr.Temporary() {
				go func() {
					n.eventCh <- ev
				}()
			}
		}
		log.Println(err)
		n.Destroy()
		return
	}
	err = encoder.Encode(ev)
	if err != nil {
		if netErr, ok := err.(net.Error); ok {
			if netErr.Temporary() {
				go func() {
					n.eventCh <- ev
				}()
			}
		}
		log.Println(err)
		n.Destroy()
	}
}

func StartNetworkListener() {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalln(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalln(err)
		}
		NewNetworkFrontend(conn)
	}
}

type NetworkFrontend struct {
	Network
	id string
}

func NewNetworkFrontend(conn net.Conn) *NetworkFrontend {
	n := NetworkFrontend{
		Network: Network{
			conn:    conn,
			dir:     events.DirFront,
			decoder: json.NewDecoder(conn),
			eventCh: make(chan events.Event),
			close:   make(chan struct{}),
		},
	}
	id, err := uuid.NewV4()
	if err != nil {
		log.Fatalln(err)
	}
	n.id = id.String()
	events.AddListener(n.eventCh, events.DirFront, 0)
	setEvent := &events.SetUUID{
		UUID: n.id,
	}
	n.sendEvent(setEvent)
	go n.readloop()
	go n.mainloop()
	joinEvent := &events.PlayerJoin{
		UUID: n.id,
	}
	events.SendEvent(joinEvent)
	events.SendEvent(events.ReloadLevel{})
	return &n
}

type NetworkBackend struct {
	Network
}

func NewNetworkBackend(address string) *NetworkBackend {
	n := NetworkBackend{
		Network: Network{
			dir:     events.DirSystem,
			eventCh: make(chan events.Event),
			close:   make(chan struct{}),
		},
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(address, port))
	if err != nil {
		log.Fatalln(err)
	}
	n.conn = conn
	n.decoder = json.NewDecoder(n.conn)
	events.AddListener(n.eventCh, events.DirSystem, 0)
	go n.readloop()
	go n.mainloop()
	return &n
}
