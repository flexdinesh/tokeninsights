package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

const frameInterval = 150 * time.Millisecond

type viewModel struct {
	worker          *loader
	filters         filters
	data            snapshot
	generation      uint64
	loading, loaded bool
	err, dateError  string
	externalURL     string
	list            ui.ListState
}

func newViewModel(connect connector, target string) *viewModel {
	m := &viewModel{worker: newLoader(connect, target), filters: filters{Period: "month"}}
	m.reload(false)
	return m
}

func (m *viewModel) close() { m.worker.cancel() }

func (m *viewModel) reload(retry bool) {
	if err := m.filters.validate(); err != nil {
		m.dateError = err.Error()
		return
	}
	m.dateError = ""
	m.generation++
	m.loading, m.err = true, ""
	m.worker.request(loadRequest{Generation: m.generation, Filters: m.filters, Retry: retry})
}

func (m *viewModel) receive() {
	for {
		select {
		case result := <-m.worker.results:
			if result.Generation != m.generation {
				continue
			}
			m.loading = false
			if result.Err != nil {
				var external *externalTarget
				if errors.As(result.Err, &external) {
					m.externalURL = external.URL
				}
				m.err = result.Err.Error()
				continue
			}
			m.data, m.loaded, m.err = result.Data, true, ""
		default:
			return
		}
	}
}

func (m *viewModel) view(c *ui.Context) {
	m.receive()
	if m.externalURL != "" {
		url := m.externalURL
		m.externalURL = ""
		c.OpenURLThen(url, func(err error) {
			if err != nil {
				m.err = "Cannot open browser: " + err.Error()
			}
		})
	}
	c.After(frameInterval)
	t := c.Theme()
	c.Root().Background(t.Background)
	ui.Column(c).Fill().Gap(16).Padding(20).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Wrap().Children(func() {
			ui.Text(c, "Local usage").FontSize(22).Bold()
			ui.Spacer(c).Grow(1)
			if ui.Button(c.Key("refresh"), "Refresh").Disabled(m.loading).Clicked() {
				m.reload(false)
			}
		})
		m.filterView(c)
		if m.dateError != "" {
			ui.Text(c, m.dateError).TextColor(t.Danger)
		}
		ui.Divider(c)
		if m.loaded {
			m.totalsView(c)
		}
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			message := "Connecting to local usage…"
			if m.loaded {
				message = m.data.Collection + " · " + m.data.Processing
			}
			if m.loading && m.loaded {
				message = "Refreshing saved usage… · " + message
			}
			if m.err != "" {
				message = "Cannot load usage: " + m.err
			}
			ui.Text(c, message).TextColor(t.TextMuted).Grow(1).MinWidth(0)
			if m.err != "" && ui.Button(c.Key("retry-query"), "Retry").Disabled(m.loading).Clicked() {
				m.reload(false)
			}
			if m.data.CollectionFailed && ui.Button(c.Key("retry-collection"), "Retry collection").Disabled(m.loading).Clicked() {
				m.reload(true)
			}
		})
		if m.loaded {
			ui.Text(c, m.data.Range).TextColor(t.TextMuted)
			if len(m.data.Rows) == 0 {
				ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
					ui.Text(c, "No usage matches these filters.").Bold()
					ui.Text(c, "Try another date range or clear the filters.").TextColor(t.TextMuted)
					if ui.Button(c.Key("clear-empty"), "Clear filters").Clicked() {
						m.clearFilters()
					}
				})
			} else {
				m.tableView(c)
			}
		} else {
			ui.Spacer(c).Grow(1)
		}
	})
}

func (m *viewModel) filterView(c *ui.Context) {
	ui.Row(c).Gap(12).Wrap().AlignItems(ui.End).Children(func() {
		ui.Field(c, "Date range", func() {
			if ui.Select(c.Key("period"), &m.filters.Period, []string{"today", "yesterday", "week", "month", "year", "all", "custom"}).Width(140).Changed() {
				m.filters.From, m.filters.To = "", ""
				if m.filters.Period != "custom" {
					m.reload(false)
				}
			}
		})
		if m.filters.Period == "custom" {
			ui.Field(c, "From", func() { ui.TextInput(c.Key("from"), &m.filters.From).Placeholder("YYYY-MM-DD").Width(140) })
			ui.Field(c, "To", func() { ui.TextInput(c.Key("to"), &m.filters.To).Placeholder("YYYY-MM-DD").Width(140) })
			if ui.Button(c.Key("apply"), "Apply dates").Clicked() {
				if m.filters.From == "" || m.filters.To == "" {
					m.dateError = "Enter both dates as YYYY-MM-DD."
				} else {
					m.reload(false)
				}
			}
		}
		m.dimension(c, "Provider", &m.filters.Provider, m.data.Providers)
		m.dimension(c, "Model", &m.filters.Model, m.data.Models)
		m.dimension(c, "Harness", &m.filters.Harness, m.data.Harnesses)
		if ui.Button(c.Key("clear"), "Clear filters").Clicked() {
			m.clearFilters()
		}
	})
}

func (m *viewModel) clearFilters() { m.filters = filters{Period: "all"}; m.reload(false) }

func (m *viewModel) dimension(c *ui.Context, label string, value *string, options []string) {
	ui.Field(c, label, func() {
		// Keep the all-values option distinct from a real source value named
		// "All", and retain active values absent from refreshed facets.
		choices := slices.Clone(options)
		if *value != "" && !slices.Contains(choices, *value) {
			choices = append(choices, *value)
		}
		allLabel := "All " + map[string]string{"Provider": "providers", "Model": "models", "Harness": "harnesses"}[label]
		labels := []string{allLabel}
		for _, choice := range choices {
			labels = append(labels, label+": "+choice)
		}
		selected := labels[0]
		if index := slices.Index(choices, *value); index >= 0 {
			selected = labels[index+1]
		}
		if ui.Select(c.Key(label), &selected, labels).Width(170).Tooltip(selected).Changed() {
			index := slices.Index(labels, selected)
			*value = ""
			if index > 0 {
				*value = choices[index-1]
			}
			m.reload(false)
		}
	})
}

func (m *viewModel) totalsView(c *ui.Context) {
	values := []struct {
		label string
		value int64
	}{
		{"Total tokens", m.data.Totals.Total}, {"Input", m.data.Totals.Input}, {"Output", m.data.Totals.Output},
		{"Reasoning", m.data.Totals.Reasoning}, {"Cache read", m.data.Totals.CacheRead}, {"Cache write", m.data.Totals.CacheWrite},
	}
	ui.Row(c).Gap(24).Wrap().Children(func() {
		for _, item := range values {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, item.label).TextColor(c.Theme().TextMuted)
				ui.Text(c, formatCount(item.value)).FontSize(20).Bold()
			})
		}
	})
	ui.Text(c, fmt.Sprintf("%s sessions · Confirmed usage", formatCount(m.data.Totals.Sessions))).TextColor(c.Theme().TextMuted)
}

func (m *viewModel) tableView(c *ui.Context) {
	columns := []ui.TableColumn{
		{Title: "Provider", Width: 160}, {Title: "Model", Width: 210}, {Title: "Harness", Width: 140},
		{Title: "Total", Width: 120, Align: ui.End}, {Title: "Input", Width: 110, Align: ui.End},
		{Title: "Output", Width: 100, Align: ui.End}, {Title: "Reasoning", Width: 110, Align: ui.End},
		{Title: "Cache read", Width: 120, Align: ui.End}, {Title: "Cache write", Width: 120, Align: ui.End},
	}
	ui.Table(c.Key("usage"), &m.list, columns, len(m.data.Rows), func(row, column int) {
		item := m.data.Rows[row]
		cells := []string{item.Provider, item.Model, item.Harness, formatCount(item.Tokens.Total), formatCount(item.Tokens.Input), formatCount(item.Tokens.Output), formatCount(item.Tokens.Reasoning), formatCount(item.Tokens.CacheRead), formatCount(item.Tokens.CacheWrite)}
		ui.Text(c, cells[column]).SingleLine().Tooltip(cells[column])
	}).Grow(1).MinHeight(120)
}

func formatCount(value int64) string {
	text := strconv.FormatInt(value, 10)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}
