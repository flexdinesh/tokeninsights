// Package sqlutil contains private SQL adapter mechanics, never domain contracts.
package sqlutil

import (
	"strconv"
	"strings"
)

// Bind numbers positional parameters in application-owned SQL. Both SQLite and
// PostgreSQL accept $n parameters. Quoted literals/identifiers remain untouched;
// request values are always passed separately to the driver.
func Bind(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	var quote byte
	index := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				if i+1 < len(query) && query[i+1] == quote {
					i++
					b.WriteByte(quote)
				} else {
					quote = 0
				}
			}
		} else if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(c)
		} else if c == '?' {
			index++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(index))
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
