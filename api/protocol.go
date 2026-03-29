package main

import "strings"

// StatusInfo represents parsed agent status.
type StatusInfo struct {
	AgentID   int      `json:"agent_id"`
	Raw       string   `json:"raw"`
	State     string   `json:"state"`
	Whitelist []string `json:"whitelist"`
}

// BroadcastResult holds the result of a command sent to one agent.
type BroadcastResult struct {
	AgentID  int    `json:"agent_id"`
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// parseStatusResponse parses "STATE:ISOLATED,WL:ip1;ip2" into StatusInfo.
func parseStatusResponse(resp string) StatusInfo {
	info := StatusInfo{State: "UNKNOWN", Whitelist: []string{}}

	parts := strings.SplitN(resp, ",", 2)
	for _, part := range parts {
		if strings.HasPrefix(part, "STATE:") {
			info.State = strings.SplitN(part, ":", 2)[1]
		} else if strings.HasPrefix(part, "WL:") {
			wl := strings.SplitN(part, ":", 2)[1]
			if wl != "" {
				info.Whitelist = strings.Split(wl, ";")
			}
		}
	}

	return info
}
