package config

import (
	"net"
	"net/url"
	"strings"
)

func webBaseURLProblem(raw string, production bool) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") ||
		(production && u.Scheme != "https") {
		return "WEB_BASE_URL must be a website origin without credentials, path, query or fragment (HTTPS in production)"
	}
	if net.ParseIP(u.Hostname()) == nil && !strings.Contains(strings.TrimSuffix(u.Hostname(), "."), ".") {
		return "WEB_BASE_URL must use a domain or IP address; Telegram rejects localhost (use http://127.0.0.1:3000 for local tests)"
	}
	return ""
}
