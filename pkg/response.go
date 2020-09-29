package pkg

import (
	"fmt"
	"strings"
)

func Response(body string) string {
	lines := strings.Split(strings.Replace(body, "\r\n", "\n", -1), "\n")
	resp := make(map[string]string)
	for _, line := range lines {
		if !strings.HasPrefix(line, "/ddev-live-") {
			continue
		}
		switch line {
		case "/ddev-live-ping":
			resp[line] = "ddev-live-pong"
		default:
			resp[line] = fmt.Sprintf("Unknown command: `%v`", line)
		}
	}
	var r []string
	for _, msg := range resp {
		r = append(r, msg)
	}
	return strings.Join(r, "\n\n")
}
