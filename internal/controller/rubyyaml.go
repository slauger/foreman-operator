package controller

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rubyMap is an ordered YAML mapping with Ruby symbol keys (":key:"), as read by Foreman
// and the Smart Proxy. A generic YAML marshaller would quote such keys and turn them into
// strings, which both applications ignore.
type rubyMap []rubyEntry

type rubyEntry struct {
	key   string
	value any
}

// set appends a key. Values may be string, bool, int, int32, int64, []string, or rubyMap.
func (m *rubyMap) set(key string, value any) {
	*m = append(*m, rubyEntry{key: key, value: value})
}

// String renders the document including the YAML start marker.
func (m rubyMap) String() string {
	var b strings.Builder
	b.WriteString("---\n")
	m.write(&b, 0)
	return b.String()
}

func (m rubyMap) write(b *strings.Builder, indent int) {
	pad := strings.Repeat("  ", indent)
	for _, e := range m {
		fmt.Fprintf(b, "%s:%s:", pad, e.key)
		switch v := e.value.(type) {
		case rubyMap:
			b.WriteString("\n")
			v.write(b, indent+1)
		case []string:
			if len(v) == 0 {
				b.WriteString(" []\n")
				continue
			}
			b.WriteString("\n")
			for _, item := range v {
				fmt.Fprintf(b, "%s  - %s\n", pad, scalar(item))
			}
		default:
			fmt.Fprintf(b, " %s\n", scalar(v))
		}
	}
}

// scalar renders a value. Strings are JSON-quoted, which YAML reads as double-quoted strings.
func scalar(v any) string {
	switch val := v.(type) {
	case string:
		data, _ := json.Marshal(val)
		return string(data)
	case bool, int, int32, int64:
		return fmt.Sprint(val)
	default:
		panic(fmt.Sprintf("rubyMap: unsupported value type %T", v))
	}
}
