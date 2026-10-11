package configyaml

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
)

// MergeOverrides preserves subscription nodes and comments. Only TUN is merged
// per field, with null removing a field to use the core default; other top-level values (including explicit empty lists) replace it.
func MergeOverrides(raw []byte, overrides map[string]any) ([]byte, error) {
	if len(overrides) == 0 {
		return raw, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("配置必须为 YAML 对象")
	}
	root := document.Content[0]
	var patch yaml.Node
	if err := patch.Encode(overrides); err != nil {
		return nil, err
	}
	for i := 0; i < len(patch.Content); i += 2 {
		key, value := patch.Content[i], patch.Content[i+1]
		index := -1
		for j := 0; j < len(root.Content); j += 2 {
			if root.Content[j].Value == key.Value {
				index = j
				break
			}
		}
		if key.Value == "tun" && value.Kind == yaml.MappingNode {
			var base map[string]any
			if index >= 0 {
				if err := root.Content[index+1].Decode(&base); err != nil {
					return nil, err
				}
			}
			if base == nil {
				base = map[string]any{}
			}
			var changes map[string]any
			if err := value.Decode(&changes); err != nil {
				return nil, err
			}
			for k, v := range changes {
				if v == nil {
					delete(base, k)
				} else {
					base[k] = v
				}
			}
			if err := value.Encode(base); err != nil {
				return nil, err
			}
		}
		if index < 0 {
			root.Content = append(root.Content, key, value)
			continue
		}
		previous := root.Content[index+1]
		preserveMappingOrder(previous, value)
		value.HeadComment = previous.HeadComment
		value.Anchor = previous.Anchor
		*previous = *value
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	return output.Bytes(), encoder.Close()
}

// Keep surviving keys where they were. Encode(map) sorts new keys, so the
// remaining replacement keys are appended deterministically. Sequences are
// never reordered: rule priority, group order and group members stay intact.
func preserveMappingOrder(previous, replacement *yaml.Node) {
	if previous.Kind == yaml.AliasNode {
		previous = previous.Alias
	}
	if previous == nil || previous.Kind != yaml.MappingNode || replacement.Kind != yaml.MappingNode {
		return
	}
	indices := make(map[string]int, len(replacement.Content)/2)
	for i := 0; i < len(replacement.Content); i += 2 {
		indices[replacement.Content[i].Value] = i
	}
	ordered := make([]*yaml.Node, 0, len(replacement.Content))
	for i := 0; i < len(previous.Content); i += 2 {
		key := previous.Content[i]
		if j, exists := indices[key.Value]; exists {
			preserveMappingOrder(previous.Content[i+1], replacement.Content[j+1])
			ordered = append(ordered, replacement.Content[j], replacement.Content[j+1])
			delete(indices, key.Value)
		}
	}
	for i := 0; i < len(replacement.Content); i += 2 {
		if _, exists := indices[replacement.Content[i].Value]; exists {
			ordered = append(ordered, replacement.Content[i], replacement.Content[i+1])
		}
	}
	replacement.Content = ordered
}
