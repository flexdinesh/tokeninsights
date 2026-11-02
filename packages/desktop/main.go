// Command tokeninsights-desktop is a thin viewer for TokenInsights dashboards.
//
// It owns no usage storage. The CLI serves a dashboard and hands its URL over
// as a tokeninsights://open?url=... link; the viewer shows it in a window.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/desktop/internal/link"
	"github.com/flexdinesh/tokeninsights/packages/desktop/internal/trust"
)

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	v := newViewer(trustStore())
	mygo.App.OnOpenURL(v.open)
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if !hasLink(args) {
			v.focus()
		}
	})
	mygo.App.WhenReady(v.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func trustStore() *trust.Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	store := trust.New(filepath.Join(dir, "tokeninsights-desktop", "trusted-origins.json"))
	return &store
}

func hasLink(args []string) bool {
	for _, arg := range args {
		if _, err := link.Parse(arg); err == nil {
			return true
		}
	}
	return false
}
