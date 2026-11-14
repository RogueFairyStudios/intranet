package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	reSafeYAMLKey    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	reSafeYAMLString = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
)

type yamlKind int

const (
	yamlNull yamlKind = iota
	yamlBool
	yamlNumber
	yamlString
	yamlSeq
	yamlMap
)

type yamlEntry struct {
	key string
	val yamlNode
}

type yamlNode struct {
	kind    yamlKind
	boolVal bool
	strVal  string
	seq     []yamlNode
	entries []yamlEntry
}

// ParseConfigFormat normalizes a requested output format ("json" or "yaml") from explicit query/extension or Accept header.
func ParseConfigFormat(explicitFormat, extFormat, acceptHeader string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(explicitFormat))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(extFormat))
	}
	if raw != "" {
		switch raw {
		case "json":
			return "json", nil
		case "yaml", "yml":
			return "yaml", nil
		default:
			return "", errBadInput("Unsupported configuration format (expected json or yaml)")
		}
	}
	acc := strings.ToLower(acceptHeader)
	if strings.Contains(acc, "application/yaml") ||
		strings.Contains(acc, "application/x-yaml") ||
		strings.Contains(acc, "text/yaml") ||
		strings.Contains(acc, "text/x-yaml") {
		return "yaml", nil
	}
	return "json", nil
}

// MarshalConfig encodes value in deterministic JSON or YAML (preserving struct field order).
func MarshalConfig(value any, format string, headerComments ...string) ([]byte, string, error) {
	normFmt, err := ParseConfigFormat(format, "", "")
	if err != nil {
		return nil, "", err
	}

	var jsonBuf bytes.Buffer
	enc := json.NewEncoder(&jsonBuf)
	enc.SetEscapeHTML(false)
	if normFmt == "json" {
		enc.SetIndent("", "  ")
		if err := enc.Encode(value); err != nil {
			return nil, "", err
		}
		return jsonBuf.Bytes(), "application/json; charset=utf-8", nil
	}

	if err := enc.Encode(value); err != nil {
		return nil, "", err
	}
	root, err := decodeOrderedJSON(jsonBuf.Bytes())
	if err != nil {
		return nil, "", err
	}

	var out strings.Builder
	for _, line := range headerComments {
		clean := strings.ReplaceAll(strings.ReplaceAll(line, "\r", ""), "\n", " ")
		if strings.TrimSpace(clean) != "" {
			out.WriteString("# ")
			out.WriteString(clean)
			out.WriteByte('\n')
		}
	}
	writeYAMLRoot(&out, root)
	return []byte(out.String()), "application/yaml; charset=utf-8", nil
}

func decodeOrderedJSON(data []byte) (yamlNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return readJSONValue(dec)
}

func readJSONValue(dec *json.Decoder) (yamlNode, error) {
	tok, err := dec.Token()
	if err != nil {
		return yamlNode{}, err
	}
	return readJSONToken(dec, tok)
}

func readJSONToken(dec *json.Decoder, tok json.Token) (yamlNode, error) {
	switch v := tok.(type) {
	case nil:
		return yamlNode{kind: yamlNull}, nil
	case bool:
		return yamlNode{kind: yamlBool, boolVal: v}, nil
	case json.Number:
		return yamlNode{kind: yamlNumber, strVal: v.String()}, nil
	case string:
		return yamlNode{kind: yamlString, strVal: v}, nil
	case json.Delim:
		switch v {
		case '[':
			var items []yamlNode
			for dec.More() {
				child, err := readJSONValue(dec)
				if err != nil {
					return yamlNode{}, err
				}
				items = append(items, child)
			}
			endTok, err := dec.Token()
			if err != nil || endTok != json.Delim(']') {
				return yamlNode{}, io.ErrUnexpectedEOF
			}
			return yamlNode{kind: yamlSeq, seq: items}, nil
		case '{':
			var entries []yamlEntry
			for dec.More() {
				kTok, err := dec.Token()
				if err != nil {
					return yamlNode{}, err
				}
				key, ok := kTok.(string)
				if !ok {
					return yamlNode{}, fmt.Errorf("expected string key in JSON object")
				}
				val, err := readJSONValue(dec)
				if err != nil {
					return yamlNode{}, err
				}
				entries = append(entries, yamlEntry{key: key, val: val})
			}
			endTok, err := dec.Token()
			if err != nil || endTok != json.Delim('}') {
				return yamlNode{}, io.ErrUnexpectedEOF
			}
			return yamlNode{kind: yamlMap, entries: entries}, nil
		}
	}
	return yamlNode{}, fmt.Errorf("unexpected JSON token %T", tok)
}

func writeYAMLRoot(b *strings.Builder, n yamlNode) {
	switch n.kind {
	case yamlMap:
		if len(n.entries) == 0 {
			b.WriteString("{}\n")
			return
		}
		for _, e := range n.entries {
			writeYAMLMapEntry(b, "", 0, e.key, e.val)
		}
	case yamlSeq:
		if len(n.seq) == 0 {
			b.WriteString("[]\n")
			return
		}
		writeYAMLSeq(b, 0, n.seq)
	default:
		b.WriteString(formatYAMLScalar(n))
		b.WriteByte('\n')
	}
}

func writeYAMLMapEntry(b *strings.Builder, prefix string, keyCol int, key string, val yamlNode) {
	b.WriteString(prefix)
	b.WriteString(formatYAMLKey(key))
	switch val.kind {
	case yamlMap:
		if len(val.entries) == 0 {
			b.WriteString(": {}\n")
			return
		}
		b.WriteString(":\n")
		childIndent := strings.Repeat(" ", keyCol+2)
		for _, ce := range val.entries {
			writeYAMLMapEntry(b, childIndent, keyCol+2, ce.key, ce.val)
		}
	case yamlSeq:
		if len(val.seq) == 0 {
			b.WriteString(": []\n")
			return
		}
		b.WriteString(":\n")
		writeYAMLSeq(b, keyCol+2, val.seq)
	default:
		b.WriteString(": ")
		b.WriteString(formatYAMLScalar(val))
		b.WriteByte('\n')
	}
}

func writeYAMLSeq(b *strings.Builder, seqIndent int, items []yamlNode) {
	pad := strings.Repeat(" ", seqIndent)
	for _, item := range items {
		switch item.kind {
		case yamlMap:
			if len(item.entries) == 0 {
				b.WriteString(pad)
				b.WriteString("- {}\n")
				continue
			}
			first := item.entries[0]
			writeYAMLMapEntry(b, pad+"- ", seqIndent+2, first.key, first.val)
			subPad := strings.Repeat(" ", seqIndent+2)
			for _, rest := range item.entries[1:] {
				writeYAMLMapEntry(b, subPad, seqIndent+2, rest.key, rest.val)
			}
		case yamlSeq:
			if len(item.seq) == 0 {
				b.WriteString(pad)
				b.WriteString("- []\n")
				continue
			}
			b.WriteString(pad)
			b.WriteString("-\n")
			writeYAMLSeq(b, seqIndent+2, item.seq)
		default:
			b.WriteString(pad)
			b.WriteString("- ")
			b.WriteString(formatYAMLScalar(item))
			b.WriteByte('\n')
		}
	}
}

func isYAMLReservedWord(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "null", "yes", "no", "on", "off", "y", "n", "~", ".nan", ".inf", "-.inf", "+.inf":
		return true
	default:
		return false
	}
}

func formatYAMLKey(k string) string {
	if k != "" && len(k) <= 80 && reSafeYAMLKey.MatchString(k) && !isYAMLReservedWord(k) {
		return k
	}
	return quoteYAMLString(k)
}

func formatYAMLScalar(n yamlNode) string {
	switch n.kind {
	case yamlNull:
		return "null"
	case yamlBool:
		if n.boolVal {
			return "true"
		}
		return "false"
	case yamlNumber:
		return n.strVal
	case yamlString:
		s := n.strVal
		if s != "" && len(s) <= 80 && reSafeYAMLString.MatchString(s) && !isYAMLReservedWord(s) {
			return s
		}
		return quoteYAMLString(s)
	default:
		return "null"
	}
}

func quoteYAMLString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

