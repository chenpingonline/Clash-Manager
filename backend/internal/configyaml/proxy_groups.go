package configyaml

import "gopkg.in/yaml.v3"

// ProxyGroupOrder reads the YAML sequence rather than relying on indentation
// or field order. Both the Helper and Web fallback use the active file's order.
func ProxyGroupOrder(raw string) []string {
	var config struct {
		Groups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		return nil
	}
	order, seen := []string{}, map[string]bool{}
	for _, group := range config.Groups {
		if group.Name != "" && !seen[group.Name] {
			order = append(order, group.Name)
			seen[group.Name] = true
		}
	}
	return order
}
