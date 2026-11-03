# TokenInsights MyGo

Native Operate surface. Toolkit system theme, system interface font, standard
keyboard focus and labeled controls. Flat workspace with compact controls;
static totals share a row, followed by collection/processing text and a native
table. Controls wrap at the minimum 720×520 window. The table scrolls in both
directions and exposes full cell text in tooltips. Numeric columns align right.

Use a 20 DIP window inset, 16 DIP section gap, 12 DIP control gap and 24 DIP
readout gap. Heading 22 DIP; readouts 20 DIP. Theme owns all colors, borders,
focus, hover, selection and reduced-motion behavior. No additional animation.

Filters and refresh only query. Date presets apply immediately; custom dates
require both calendar-valid endpoints and Apply dates. Failed queries retain
saved totals/table. Collection errors expose Retry collection independently.
Loading never blocks input or window closure. Hosted/external targets use the
system browser. Closing a window cancels its worker and releases its lease;
reopening acquires a fresh session. No webviews, HTML or trusted-origin storage.
