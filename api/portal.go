package main

import (
	"fmt"
	"log"
	"net"
	"sync"
)

// Portal manages TCP connections from eBPF agents.
type Portal struct {
	port     int
	agents   map[int]*Agent
	nextID   int
	mu       sync.RWMutex
	listener net.Listener
}

// NewPortal creates a new Portal instance.
func NewPortal(port int) *Portal {
	return &Portal{
		port:   port,
		agents: make(map[int]*Agent),
		nextID: 1,
	}
}

// Start binds the TCP listener and begins accepting agent connections.
func (p *Portal) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p.port))
	if err != nil {
		return err
	}
	p.listener = ln
	log.Printf("TCP server listening on 0.0.0.0:%d", p.port)

	go p.acceptLoop()
	return nil
}

func (p *Portal) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return // listener closed
		}

		p.mu.Lock()
		id := p.nextID
		p.nextID++
		agent := NewAgent(conn, id)
		p.agents[id] = agent
		p.mu.Unlock()

		log.Printf("Agent #%d connected from %s", id, agent.Hostname)
	}
}

// GetAgents returns a list of all connected agents.
func (p *Portal) GetAgents() []AgentInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	list := make([]AgentInfo, 0, len(p.agents))
	for _, agent := range p.agents {
		list = append(list, agent.ToJSON())
	}
	return list
}

// SendCommand sends a command to a specific agent by ID.
func (p *Portal) SendCommand(agentID int, cmd string) (string, error) {
	p.mu.RLock()
	agent, ok := p.agents[agentID]
	p.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("Agent #%d not found", agentID)
	}

	resp, err := agent.Send(cmd)
	if err != nil {
		p.removeAgent(agentID)
		return "", fmt.Errorf("Agent #%d disconnected", agentID)
	}

	return resp, nil
}

// BroadcastCommand sends a command to all connected agents.
func (p *Portal) BroadcastCommand(cmd string) []BroadcastResult {
	p.mu.RLock()
	snapshot := make([]*Agent, 0, len(p.agents))
	ids := make([]int, 0, len(p.agents))
	for id, agent := range p.agents {
		ids = append(ids, id)
		snapshot = append(snapshot, agent)
	}
	p.mu.RUnlock()

	var results []BroadcastResult
	var disconnected []int

	for i, agent := range snapshot {
		resp, err := agent.Send(cmd)
		if err != nil {
			results = append(results, BroadcastResult{
				AgentID: ids[i], Error: "disconnected",
			})
			disconnected = append(disconnected, ids[i])
		} else {
			results = append(results, BroadcastResult{
				AgentID: ids[i], Response: resp,
			})
		}
	}

	for _, id := range disconnected {
		p.removeAgent(id)
	}

	return results
}

func (p *Portal) removeAgent(id int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if agent, ok := p.agents[id]; ok {
		agent.Close()
		delete(p.agents, id)
		log.Printf("Agent #%d removed (disconnected)", id)
	}
}

// Shutdown closes all agent connections and the TCP listener.
func (p *Portal) Shutdown() {
	p.mu.Lock()
	for _, agent := range p.agents {
		agent.Close()
	}
	p.agents = make(map[int]*Agent)
	p.mu.Unlock()

	if p.listener != nil {
		p.listener.Close()
	}
	log.Println("TCP server shutdown complete")
}
