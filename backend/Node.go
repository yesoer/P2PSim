package backend

import (
	"bytes"
	"distributed-sys-emulator/bus"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"golang.org/x/net/context"
)

type Node interface {
	Send(targetId int, data any) int
	Await(cnt int) int
	ConnectTo(peerId int)
	DisconnectFrom(peerId int)
	GetConnections() []*Connection
	Run(eb *bus.EventBus, signals chan Signal)
	SetData(json interface{})
	GetData() interface{}
}

type Connection struct {
	Chan   chan interface{}
	Target int
}

type node struct {
	connections []*Connection // TODO : I don't like the way these work
	id          int           // TODO : still don't know about this, keep ids implicit or explicit everywhere
	data        interface{}   // json data to expose to lua
}

func NewNode(id int) Node {
	var connections []*Connection
	return &node{connections, id, nil}
}

func (n *node) SetData(json interface{}) {
	n.data = json
}

func (n *node) GetData() interface{} {
	return n.data
}

// function to be used from lua to send a message (data is the first parameter)
// to a specific node
// TODO : another one to send to all
// TODO : another one to provide equation, send to all that resolve it e.g. for all even id's
func (n *node) Send(targetId int, data any) int {
	for _, c := range n.connections {
		if c.Target == targetId {
			c.Chan <- data
			return 0
		}
	}
	return 0
}

// function to be used from lua to wait for n messages from all connected peers
func (n *node) Await(cnt int) int {
	var wg sync.WaitGroup
	wg.Add(cnt)
	// channel to kill those channels where we don't expect a message ?
	kill := make(chan bool, 10)

	// listen on all channels until the specified number of messages is reached
	res := []interface{}{}
	for _, c := range n.connections {
		go func(c *Connection, wg *sync.WaitGroup) {
			for {
				select {
				case msg := <-c.Chan:
					res = append(res, msg)
					wg.Done()
				case <-kill:
					return
				}
			}
		}(c, &wg)
	}

	wg.Wait()
	for i := 0; i <= len(n.connections)-cnt; i++ {
		kill <- true
	}

	// TODO : return res
	return 1
}

// a node will run continuously, the current state can be changed using signals
func (n *node) Run(eb *bus.EventBus, signals chan Signal) {
	code := Code("")

	eb.Bind(bus.CodeChangedEvt, func(e bus.Event) {
		code = e.Data.(Code)
	})

	// code exec
	go func() {
		var cancel context.CancelFunc
		ctx, cancel := context.WithCancel(context.Background())
		var output string
		exec := func() {
			var stdout, stderr bytes.Buffer
			i := interp.New(interp.Options{Stdout: &stdout, Stderr: &stderr})

			if err := i.Use(stdlib.Symbols); err != nil {
				panic(err)
			}

			_, err := i.Eval(string(code))
			if err != nil {
				panic(err)
			}

			v, err := i.Eval("Run")
			if err != nil {
				fmt.Println("Error ", err)
				return
			}

			// TODO : accept empty interface as return/do we even need returns ?
			userF := v.Interface().(func(context.Context, func(targetId int, data any) int, func(int) int) string)

			// make node specific data accessible
			ctx = context.WithValue(ctx, "node", n.data)

			// Execute the provided function
			userF(ctx, n.Send, n.Await)

			output = stdout.String()
		}

		// wait for other signals
		running := false
		for sig := range signals {
			log.Println("Node ", n.id, " received signal ", sig)
			switch sig {
			case START:
				if !running {
					go exec()
					running = true
				}
			case STOP:
				if running {
					// kill exec of userF and return to start of loop
					cancel()
					running = false
					// TODO : this waiting for userF to cancel sucks
					time.Sleep(time.Second * 10)
					data := bus.Output{Str: output, NodeId: n.id}
					e := bus.Event{Type: bus.OutputChanged, Data: data}
					eb.Publish(e)
				}
			case TERM:
				if running {
					cancel()
				}
				return
			}
		}
	}()
}

type userFunc func(context.Context, func(targetId int, data any) int, func(int) int) string

// make a one way connection from  n to peer, meaning peer adds n's output as
// input
func (n *node) ConnectTo(peerId int) {
	c := make(chan interface{}, 10)
	newConnection := Connection{c, peerId}
	n.connections = append(n.connections, &newConnection)
}

func (n *node) DisconnectFrom(peerId int) {
	for connI, conn := range n.connections {
		if conn.Target == peerId {
			n.connections = append(n.connections[:connI], n.connections[connI+1:]...)
			return
		}
	}
}

func (n *node) GetConnections() []*Connection {
	return n.connections
}
