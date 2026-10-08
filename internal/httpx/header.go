package httpx

import (
	"net/textproto"
	"strings"
)

// Header is an ordered multi-valued header map. Keys are canonical MIME names.
// Responses carry about a dozen headers, so a slice scan beats a map for both
// speed and allocation. A nil *Header reads as empty.
type Header struct{ fields []field }

type field struct {
	key   string
	value string
	more  []string
}

// CanonicalKey returns the canonical MIME spelling of a header name.
func CanonicalKey(key string) string { return textproto.CanonicalMIMEHeaderKey(key) }

func (h *Header) find(key string) *field {
	if h == nil {
		return nil
	}
	for i := range h.fields {
		if h.fields[i].key == key {
			return &h.fields[i]
		}
	}
	return nil
}

// Get returns the first value of the header, or "".
func (h *Header) Get(key string) string {
	if f := h.find(CanonicalKey(key)); f != nil {
		return f.value
	}
	return ""
}

// Values returns a copy of every value of the header.
func (h *Header) Values(key string) []string {
	f := h.find(CanonicalKey(key))
	if f == nil {
		return nil
	}
	values := make([]string, 0, 1+len(f.more))
	values = append(values, f.value)
	return append(values, f.more...)
}

// Lookup reports whether the header is present, and returns its values.
func (h *Header) Lookup(key string) ([]string, bool) {
	values := h.Values(key)
	return values, values != nil
}

// Has reports whether the header is present.
func (h *Header) Has(key string) bool { return h.find(CanonicalKey(key)) != nil }

// Set replaces every value of the header.
func (h *Header) Set(key, value string) {
	key = CanonicalKey(key)
	if f := h.find(key); f != nil {
		f.value, f.more = value, nil
		return
	}
	h.fields = append(h.fields, field{key: key, value: value})
}

// Add appends a value to the header.
func (h *Header) Add(key, value string) {
	key = CanonicalKey(key)
	if f := h.find(key); f != nil {
		f.more = append(f.more, value)
		return
	}
	h.fields = append(h.fields, field{key: key, value: value})
}

// SetValues replaces the header with exactly the given values. No values removes it.
func (h *Header) SetValues(key string, values []string) {
	if len(values) == 0 {
		h.Del(key)
		return
	}
	h.Set(key, values[0])
	if len(values) > 1 {
		h.find(CanonicalKey(key)).more = append([]string(nil), values[1:]...)
	}
}

// Del removes the header.
func (h *Header) Del(key string) {
	if h == nil {
		return
	}
	key = CanonicalKey(key)
	for i := range h.fields {
		if h.fields[i].key == key {
			h.fields = append(h.fields[:i], h.fields[i+1:]...)
			return
		}
	}
}

// Len returns the number of distinct header names.
func (h *Header) Len() int {
	if h == nil {
		return 0
	}
	return len(h.fields)
}

// Each calls visit for every name/value pair in insertion order.
func (h *Header) Each(visit func(key, value string)) {
	if h == nil {
		return
	}
	for _, f := range h.fields {
		visit(f.key, f.value)
		for _, value := range f.more {
			visit(f.key, value)
		}
	}
}

// Clone returns an independent copy.
func (h *Header) Clone() *Header {
	if h == nil {
		return nil
	}
	c := &Header{fields: make([]field, len(h.fields))}
	for i, f := range h.fields {
		c.fields[i] = field{key: f.key, value: f.value, more: append([]string(nil), f.more...)}
	}
	return c
}

// CopyFrom replaces h with a copy of src.
func (h *Header) CopyFrom(src *Header) {
	h.fields = h.fields[:0]
	if src == nil {
		return
	}
	for _, f := range src.fields {
		h.fields = append(h.fields, field{key: f.key, value: f.value, more: append([]string(nil), f.more...)})
	}
}

// Replace sets every header of src on h, replacing values of the same name.
func (h *Header) Replace(src *Header) {
	if src == nil {
		return
	}
	for _, f := range src.fields {
		h.Set(f.key, f.value)
		if len(f.more) > 0 {
			h.find(f.key).more = append([]string(nil), f.more...)
		}
	}
}

// Reset removes every header, retaining storage.
func (h *Header) Reset() {
	clear(h.fields)
	h.fields = h.fields[:0]
}

// ContainsToken reports whether any value of the header lists the token,
// comparing case-insensitively across comma-separated elements.
func (h *Header) ContainsToken(key, token string) bool {
	f := h.find(CanonicalKey(key))
	if f == nil {
		return false
	}
	if listsToken(f.value, token) {
		return true
	}
	for _, value := range f.more {
		if listsToken(value, token) {
			return true
		}
	}
	return false
}

func listsToken(value, token string) bool {
	for _, element := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(element), token) {
			return true
		}
	}
	return false
}
