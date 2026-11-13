package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/preferences"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// appearanceMenuIDs name the View menu's radio item per appearance.
var appearanceMenuIDs = map[theme.Appearance]string{
	theme.AppearanceSystem: "appearance-system",
	theme.AppearanceLight:  "appearance-light",
	theme.AppearanceDark:   "appearance-dark",
}

// appearanceMenuLabels are the View menu's labels per appearance.
var appearanceMenuLabels = map[theme.Appearance]string{
	theme.AppearanceSystem: "System Theme",
	theme.AppearanceLight:  "Light Theme",
	theme.AppearanceDark:   "Dark Theme",
}

// appearance owns the app-wide theme choice on the main thread: every
// window draws it, the header's theme button and the View menu's radios
// set it and show it, and the preferences file keeps it.
type appearance struct {
	choice theme.Appearance
	// store is nil when there is nowhere to keep the choice.
	store *preferences.Store
	// menu holds the radio items, once built.
	menu *mygo.Menu
	// applied shows a new choice: the system source and every window.
	applied func(theme.Appearance)
	// spawn runs a joined worker.
	spawn func(func())
}

// set makes choice current, shows it everywhere and persists it off the
// main thread.
func (a *appearance) set(choice theme.Appearance) {
	if !choice.Valid() {
		return
	}
	a.choice = choice
	a.syncMenu()
	a.applied(choice)
	if a.store == nil {
		return
	}
	a.store.Stage(preferences.Preferences{Appearance: choice})
	store := a.store
	a.spawn(func() {
		if err := store.Flush(); err != nil {
			log.Println(err)
		}
	})
}

// menuItems are the View menu's radio items, checked for the current
// choice.
func (a *appearance) menuItems() []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(theme.Appearances()))
	for _, choice := range theme.Appearances() {
		items = append(items, &mygo.MenuItem{
			ID: appearanceMenuIDs[choice], Label: appearanceMenuLabels[choice], Type: mygo.MenuItemRadio,
			Checked: choice == a.choice, Click: func(*mygo.MenuItem, *mygo.Window) { a.set(choice) },
		})
	}
	return items
}

// syncMenu checks the current choice's radio item.
func (a *appearance) syncMenu() {
	if a.menu == nil {
		return
	}
	for _, choice := range theme.Appearances() {
		if item := a.menu.ItemByID(appearanceMenuIDs[choice]); item != nil {
			item.SetChecked(choice == a.choice)
		}
	}
}

// themeSource is the system appearance source for a choice, so native
// chrome follows it too.
func themeSource(choice theme.Appearance) mygo.ThemeSource {
	switch choice {
	case theme.AppearanceLight:
		return mygo.ThemeLight
	case theme.AppearanceDark:
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// openPreferences reads the saved preferences from the app's user data
// directory. Any failure keeps the defaults; a missing directory keeps
// choices for this run only.
func openPreferences() (*preferences.Store, preferences.Preferences) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Println("preferences:", err)
		return nil, preferences.Default()
	}
	store, saved, err := preferences.Open(dir)
	if err != nil {
		log.Println(err)
	}
	return store, saved
}
