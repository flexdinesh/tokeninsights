package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Option is one choice of a Select or Segmented: its value and its label.
type Option[T comparable] struct {
	Value T
	Label string
}

// labelOf returns the label of value among options, or "" when none has it.
func labelOf[T comparable](options []Option[T], value T) string {
	if i := indexOf(options, value); i >= 0 {
		return options[i].Label
	}
	return ""
}

// indexOf returns the place of value among options, or -1.
func indexOf[T comparable](options []Option[T], value T) int {
	for i, o := range options {
		if o.Value == value {
			return i
		}
	}
	return -1
}

// Select builds a drop-down choosing one of options into selected, on
// ui.SelectBase: the trigger shows the choice and a chevron, and the menu
// marks the choice with a check. Give it a key with c.Key and a Label naming
// it; query Changed on the result, the trigger.
func Select[T comparable](c *ui.Context, selected *T, options []Option[T], size Size) ui.Element {
	sel := ui.SelectBase(c, selected)
	t := theme.Current(c)
	m := size.metrics()
	trigger := sel.Trigger
	trigger.MinHeight(m.height).PaddingX(m.padX).PaddingY(theme.Unit).Gap(theme.Unit*2).Justify(ui.SpaceBetween).
		Border(theme.Rule, t.LineStrong).TextColor(t.Ink)
	m.text(trigger)
	focusRing(c, trigger, trigger.FocusVisible())
	surface := t.Surface
	if (trigger.Hovered() || sel.Open()) && !trigger.IsDisabled() {
		surface = t.SurfaceHover
	}
	trigger.Background(fade(c, trigger, "bg", surface))
	width := trigger.LayoutBounds().W
	choice := labelOf(options, *selected)
	trigger.Value(choice).Children(func() {
		m.text(ui.Text(c, choice).SingleLine())
		icon(c, icons.ChevronDown()).TextColor(t.InkSubtle)
	})
	sel.Popup(func(panel ui.Element) {
		overlay(t, panel).Margin(theme.MenuOffset, 0, 0, 0).Padding(theme.Unit).MinWidth(max(theme.MenuMinWidth, width))
		for _, o := range options {
			item := sel.Item(o.Value).Gap(theme.Unit).MinHeight(theme.ControlCompact).Padding(theme.Unit, theme.Unit*2)
			if item.Highlighted() {
				item.Background(t.SurfaceHover).TextColor(t.Ink)
			}
			theme.Body(item).Children(func() {
				check := icon(c, icons.Check())
				if o.Value != *selected {
					check.Opacity(0)
				}
				ui.Text(c, o.Label).SingleLine()
			})
		}
	})
	return trigger
}

// Segmented builds an exclusive choice of options as a row of buttons in the
// look of s (Quiet or Outline, Compact or Default), the choice in the
// selection tint. label names the group for assistive technology, which
// reads it as a radio group. Give it a key with c.Key; query Changed on the
// result, the track.
func Segmented[T comparable](c *ui.Context, label string, selected *T, options []Option[T], s ButtonStyle) ui.Element {
	shown := indexOf(options, *selected)
	index := shown
	seg := ui.SegmentedBase(c, &index, len(options))
	track := seg.Track.Wrap().Gap(theme.Unit).Label(label)
	track.Children(func() {
		for i, o := range options {
			style := s
			style.Selected = i == shown
			segment := seg.Segment(i).Checked(style.Selected)
			underline := styleButton(c, segment, style)
			segment.Children(func() {
				m := s.Size.metrics()
				text := m.text(ui.Text(c, o.Label).SingleLine())
				if underline {
					text.Underline().UnderlineOffset(theme.UnderlineOffset * m.font)
				}
			})
		}
	})
	// The track takes its input with its segments: a new choice is known now.
	// The base clamps an index outside the options, which is no choice.
	if track.Changed() && index >= 0 && index < len(options) {
		*selected = options[index].Value
	}
	return track
}
