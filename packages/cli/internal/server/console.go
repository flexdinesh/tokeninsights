package server

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/muesli/termenv"
)

const logIndent = "  "

type console struct {
	output  io.Writer
	profile termenv.Profile
}

func newConsole(output io.Writer) console {
	terminal := termenv.NewOutput(output)
	return console{output: output, profile: terminal.ColorProfile()}
}

func (c console) startup(hostname, url string) error {
	brand := c.profile.String("tokeninsights " + version.Version).Bold().Foreground(c.profile.Color("#D290E4")).String()
	hostnameLabel := c.label("hostname:")
	urlLabel := c.label("url:")
	hostnameValue := c.profile.String(hostname).Foreground(c.profile.Color("#66C2CD")).String()
	urlValue := c.profile.String(url).Foreground(c.profile.Color("#A8CC8C")).Underline().String()
	stop := c.profile.String("ctrl-c to stop.").Faint().String()
	_, err := fmt.Fprintf(c.output, "%s%s\n%s%s  %s\n%s%s  %s\n\n%s%s\n", logIndent, brand, logIndent, hostnameLabel, hostnameValue, logIndent, urlLabel, urlValue, logIndent, stop)
	return err
}

func (c console) portBusy(port int, processes []portProcess) error {
	message := fmt.Sprintf("port %d is busy", port)
	if len(processes) > 0 {
		details := make([]string, 0, len(processes))
		for _, process := range processes {
			details = append(details, fmt.Sprintf("%s, pid %d", process.name, process.pid))
		}
		message += " (" + strings.Join(details, "; ") + ")"
	}
	message = c.profile.String(message + ".").Foreground(c.profile.Color("#DBAB79")).String()
	_, err := fmt.Fprintln(c.output, logIndent+message)
	return err
}

func (c console) confirmTermination(input io.Reader) (bool, error) {
	question := c.profile.String("terminate the process and start tokeninsights? [y/N] ").Foreground(c.profile.Color("#E88388")).String()
	if _, err := fmt.Fprint(c.output, logIndent+question); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func machineHostname() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "unknown"
	}
	return strings.TrimSpace(hostname)
}

func (c console) label(value string) string {
	return c.profile.String(value).Faint().String()
}
