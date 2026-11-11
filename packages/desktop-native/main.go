// Command tokeninsights-native presents TokenInsights in native MyGo UI.
// It is an experimental edge product beside TokenInsights Desktop.
package main

import (
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/session"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

func main() {
	mygo.App.SetName(appName)
	if err := validateArguments(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if err := theme.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	v := newViewer(session.Connect)
	mygo.App.SetMenu(v.menu())
	mygo.App.OnOpenURL(v.open)
	mygo.App.OnSecondInstance(func(args []string, _ string) { v.openArguments(args) })
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
		v.openArguments(os.Args[1:])
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	v.close()
}

func validateArguments(args []string) error {
	for _, arg := range args {
		if _, err := desktoplaunch.ID(browser.NativeApp, arg); err != nil {
			return errors.New("use tokeninsights connect URI --app=native to select a native connection")
		}
	}
	return nil
}
