package openapi

import "encoding/json"

// ToJSON mengubah dokumen ke JSON dengan urutan kunci asli (Map.MarshalJSON).
func ToJSON(doc Document) ([]byte, error) { return json.Marshal(doc.Root) }
