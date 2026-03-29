package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	isolationState int            // 0 = NORMAL, 1 = ISOLATED
	whitelist      map[string]bool
)

func init() {
	whitelist = make(map[string]bool)
}

// handleCommand processes a command and returns a response (same format as real C agent).
func handleCommand(cmdStr string) string {
	tokens := strings.Fields(cmdStr)
	if len(tokens) == 0 {
		return "ERR:EMPTY_COMMAND"
	}

	cmd := strings.ToLower(tokens[0])

	switch cmd {
	case "isolate":
		ips := tokens[1:]
		whitelist = make(map[string]bool)
		for _, ip := range ips {
			whitelist[ip] = true
		}
		isolationState = 1
		log.Printf("[ACTION] ISOLATED — whitelist: %v", ips)
		return fmt.Sprintf("OK:ISOLATED (%d IPs whitelisted)", len(whitelist))

	case "release":
		isolationState = 0
		whitelist = make(map[string]bool)
		log.Println("[ACTION] RELEASED")
		return "OK:RELEASED"

	case "status":
		state := "NORMAL"
		if isolationState == 1 {
			state = "ISOLATED"
		}
		wlList := sortedKeys(whitelist)
		wlStr := strings.Join(wlList, ";")
		log.Printf("[ACTION] STATUS query — state=%s", state)
		return fmt.Sprintf("STATE:%s,WL:%s", state, wlStr)

	case "whitelist":
		if len(tokens) < 3 {
			return "ERR:USAGE whitelist add|del <ip>"
		}
		action := strings.ToLower(tokens[1])
		ip := tokens[2]
		switch action {
		case "add":
			whitelist[ip] = true
			log.Printf("[ACTION] Whitelist ADD %s", ip)
			return fmt.Sprintf("OK:WL_ADD %s", ip)
		case "del":
			delete(whitelist, ip)
			log.Printf("[ACTION] Whitelist DEL %s", ip)
			return fmt.Sprintf("OK:WL_DEL %s", ip)
		default:
			return "ERR:USAGE whitelist add|del <ip>"
		}

	case "quit":
		log.Println("[ACTION] QUIT received — shutting down")
		return "OK:SHUTDOWN"

	default:
		return fmt.Sprintf("ERR:UNKNOWN_COMMAND %s", cmd)
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func connectToPortal(host string, port int) {
	addr := fmt.Sprintf("%s:%d", host, port)

	for {
		log.Printf("Connecting to portal %s ...", addr)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log.Printf("Portal not ready, retrying in 5s ...")
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("Connected to portal %s", addr)

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			log.Printf("Received: %s", line)
			response := handleCommand(line)
			log.Printf("Response: %s", response)
			_, err := conn.Write([]byte(response + "\n"))
			if err != nil {
				log.Printf("Write error: %v", err)
				break
			}

			if strings.ToLower(line) == "quit" {
				conn.Close()
				os.Exit(0)
			}
		}

		log.Println("Portal disconnected.")
		conn.Close()
		time.Sleep(5 * time.Second)
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[AGENT] ")

	host := os.Getenv("PORTAL_HOST")
	if host == "" {
		host = "portal"
	}
	port := 9999
	if p := os.Getenv("PORTAL_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	log.Println("Mock agent starting")
	log.Printf("Target portal: %s:%d", host, port)
	connectToPortal(host, port)
}
