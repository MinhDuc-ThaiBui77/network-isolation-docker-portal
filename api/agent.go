package main

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AgentInfo is the JSON-serializable representation of an Agent.
type AgentInfo struct {
	ID          int    `json:"id"`
	Hostname    string `json:"hostname"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	ConnectedAt string `json:"connected_at"`
}

// Agent represents a single connected eBPF agent.
type Agent struct {
	ID          int
	Conn        net.Conn
	IP          string
	Port        int
	Hostname    string
	ConnectedAt string
	mu          sync.Mutex
}

// NewAgent creates an Agent from an accepted TCP connection.
func NewAgent(conn net.Conn, id int) *Agent {
	host, portStr, _ := net.SplitHostPort(conn.RemoteAddr().String())
	port, _ := strconv.Atoi(portStr)

	return &Agent{
		ID:          id,
		Conn:        conn,
		IP:          host,
		Port:        port,
		Hostname:    conn.RemoteAddr().String(),
		ConnectedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
}

// Send sends a command to the agent and waits for a response (5s timeout).
func (a *Agent) Send(msg string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, err := a.Conn.Write([]byte(strings.TrimSpace(msg) + "\n"))
	if err != nil {
		return "", err
	}

	a.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, err := a.Conn.Read(buf)
	a.Conn.SetReadDeadline(time.Time{}) // reset deadline
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(buf[:n])), nil
}

// ToJSON returns the JSON-serializable info for this agent.
func (a *Agent) ToJSON() AgentInfo {
	return AgentInfo{
		ID:          a.ID,
		Hostname:    a.Hostname,
		IP:          a.IP,
		Port:        a.Port,
		ConnectedAt: a.ConnectedAt,
	}
}

// Close closes the agent's TCP connection.
func (a *Agent) Close() {
	a.Conn.Close()
}
