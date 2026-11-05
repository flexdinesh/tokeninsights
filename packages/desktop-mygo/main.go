// Command tokeninsights-mygo presents the shared web dashboard in a MyGo webview window.
package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
)

func main() {
	mygo.App.SetName(appName)
	if err := validateArguments(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	v := newViewer(func(ctx context.Context, target string) (*dashboard, error) {
		return connectDashboard(ctx, target, nil)
	})
	mygo.App.OnOpenURL(v.open)
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if !hasLink(args) {
			v.focus()
		} else {
			for _, arg := range args {
				if _, err := desktoplaunch.ID(arg); err == nil {
					v.open(arg)
				}
			}
		}
	})
	initialized := false
	mygo.App.OnActivate(func(_ bool) {
		if initialized {
			v.focus()
		}
	})
	// An idle shell keeps no session. Reopening acquires a new backend lease.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() {
		initialized = true
		if !hasLink(os.Args[1:]) {
			v.focus()
		} else {
			for _, arg := range os.Args[1:] {
				if _, err := desktoplaunch.ID(arg); err == nil {
					v.open(arg)
				}
			}
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	v.close()
}

func validateArguments(args []string) error {
	for _, arg := range args {
		if _, err := desktoplaunch.ID(arg); err != nil {
			return errors.New("use tokeninsights connect URI to select a desktop connection")
		}
	}
	return nil
}

func hasLink(args []string) bool {
	for _, arg := range args {
		if _, err := desktoplaunch.ID(arg); err == nil {
			return true
		}
	}
	return false
}
