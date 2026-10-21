package openapi

import (
	"bytes"
	"encoding/json"
)

// Map adalah map berurutan yang menjaga urutan kunci asli saat di-marshal ke JSON.
// Nilai: string, json.Number, float64, bool, nil, *Map, []any.
type Map struct {
	keys []string
	vals map[string]any
}

// NewMap membuat Map kosong.
func NewMap() *Map { return &Map{vals: map[string]any{}} }

// Set menambah atau mengganti nilai; kunci baru ditaruh paling akhir.
func (m *Map) Set(k string, v any) {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

// Get mengembalikan nilai kunci k.
func (m *Map) Get(k string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[k]
	return v, ok
}

// Keys mengembalikan salinan kunci sesuai urutan.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Len adalah jumlah kunci.
func (m *Map) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// clone menyalin satu tingkat.
func (m *Map) clone() *Map {
	c := NewMap()
	if m == nil {
		return c
	}
	for _, k := range m.keys {
		c.Set(k, m.vals[k])
	}
	return c
}

// MarshalJSON menulis kunci sesuai urutan asli; escape sama dengan encoding/json.
func (m *Map) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// asMap mengembalikan v sebagai *Map bila bertipe itu.
func asMap(v any) (*Map, bool) {
	m, ok := v.(*Map)
	return m, ok && m != nil
}
