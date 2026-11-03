// Command tokeninsights-mygo presents local usage through MyGo's native Go UI.
package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/desktop-mygo/internal/link"
)

func main() {
	mygo.App.SetName(appName)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	endpoint := endpointFlag(os.Args[1:])
	v := newViewer(func(ctx context.Context, target string, capture bool) (dataSource, error) {
		return connectLocal(ctx, target, capture, endpoint)
	})
	mygo.App.OnOpenURL(v.open)
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if !hasLink(args) {
			v.focus()
		}
	})
	mygo.App.OnActivate(func(_ bool) { v.focus() })
	// An idle shell keeps no session. Reopening acquires a new backend lease.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() {
		if !hasLink(os.Args[1:]) {
			v.focus()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	v.close()
}

// endpointFlag returns the launch --endpoint=VALUE, which overrides
// TOKENINSIGHTS_ENDPOINT and remote.endpoint for this application instance.
func endpointFlag(args []string) *string {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--endpoint="); ok {
			return &value
		}
	}
	return nil
}

func hasLink(args []string) bool {
	for _, arg := range args {
		if _, err := link.Parse(arg); err == nil {
			return true
		}
	}
	return false
}
