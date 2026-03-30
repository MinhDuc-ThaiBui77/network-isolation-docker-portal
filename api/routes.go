package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var portal *Portal

func registerRoutes(mux *http.ServeMux, p *Portal) {
	portal = p

	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /api/auth/login", loginHandler)
	mux.HandleFunc("GET /api/agents", authMiddleware(listAgentsHandler))
	mux.HandleFunc("GET /api/agents/{id}/status", authMiddleware(agentStatusHandler))
	mux.HandleFunc("POST /api/agents/{id}/isolate", authMiddleware(isolateHandler))
	mux.HandleFunc("POST /api/agents/{id}/release", authMiddleware(releaseHandler))
	mux.HandleFunc("POST /api/agents/{id}/whitelist", authMiddleware(whitelistHandler))
	mux.HandleFunc("POST /api/agents/broadcast", authMiddleware(broadcastHandler))
	mux.HandleFunc("GET /api/events", authMiddleware(eventsHandler))
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// getAgentID parses the {id} path parameter as an int.
func getAgentID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"tcp_port": portal.port,
	})
}

func listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	agents := portal.GetAgents()
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func agentStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getAgentID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	cacheKey := fmt.Sprintf("agent:status:%d", id)

	// Check cache first
	if cached, hit := CacheGet(cacheKey); hit {
		status := parseStatusResponse(cached)
		status.AgentID = id
		status.Raw = cached
		writeJSON(w, http.StatusOK, status)
		return
	}

	// Cache miss — ask agent via TCP
	resp, err := portal.SendCommand(id, "status")
	if err != nil {
		code := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	// Cache the response for 10 seconds
	CacheSet(cacheKey, resp, 10*time.Second)

	status := parseStatusResponse(resp)
	status.AgentID = id
	status.Raw = resp
	writeJSON(w, http.StatusOK, status)
}

func isolateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getAgentID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var body struct {
		IPs []string `json:"ips"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if len(body.IPs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ips list is required"})
		return
	}

	cmd := "isolate " + strings.Join(body.IPs, " ")
	resp, err := portal.SendCommand(id, cmd)
	if err != nil {
		code := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	CacheInvalidate(fmt.Sprintf("agent:status:%d", id))
	go LogEvent(id, "isolate", strings.Join(body.IPs, ","), resp)
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "response": resp})
}

func releaseHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getAgentID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	resp, err := portal.SendCommand(id, "release")
	if err != nil {
		code := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	CacheInvalidate(fmt.Sprintf("agent:status:%d", id))
	go LogEvent(id, "release", "", resp)
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "response": resp})
}

func whitelistHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getAgentID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var body struct {
		Action string `json:"action"`
		IP     string `json:"ip"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if (body.Action != "add" && body.Action != "del") || body.IP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action (add/del) and ip are required"})
		return
	}

	cmd := "whitelist " + body.Action + " " + body.IP
	resp, err := portal.SendCommand(id, cmd)
	if err != nil {
		code := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	CacheInvalidate(fmt.Sprintf("agent:status:%d", id))
	go LogEvent(id, "whitelist", body.Action+" "+body.IP, resp)
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "response": resp})
}

func broadcastHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Command string `json:"command"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	results := portal.BroadcastCommand(body.Command)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	agentID := 0
	if v := r.URL.Query().Get("agent_id"); v != "" {
		agentID, _ = strconv.Atoi(v)
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	events, err := QueryEvents(agentID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if events == nil {
		events = []Event{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
