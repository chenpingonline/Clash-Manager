package configyaml

import (
	"reflect"
	"testing"
)

func TestProxyGroupOrder(t *testing.T) {
	raw := "mixed-port: 7890\nproxy-groups:\n  - name: 节点选择\n    type: select\n  - { name: \"香港, 自动\", type: url-test }\n  - name: 'Work''s Proxy'\nrules:\n  - MATCH,节点选择\n"
	want := []string{"节点选择", "香港, 自动", "Work's Proxy"}
	if got := ProxyGroupOrder(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestProxyGroupOrderSupportsYAMLFormats(t *testing.T) {
	for label, raw := range map[string]string{
		"inline":          `proxy-groups: [{type: select, name: Z}, {name: A, type: select}]`,
		"unindented":      "proxy-groups:\n- name: Z\n  type: select\n- name: A\nrules: []\n",
		"name after type": "proxy-groups: # preserve order\n  - type: select\n    name: Z\n  - type: select\n    name: A\n",
		"anchors":         "first: &first {name: Z, type: select}\nproxy-groups: [*first, {name: A}]\n",
	} {
		t.Run(label, func(t *testing.T) {
			if got := ProxyGroupOrder(raw); !reflect.DeepEqual(got, []string{"Z", "A"}) {
				t.Fatalf("got %#v", got)
			}
		})
	}
}

func TestProxyGroupOrderHandlesInvalidAndMissingGroups(t *testing.T) {
	for _, raw := range []string{"rules: []", "proxy-groups: [", "proxy-groups: wrong"} {
		if got := ProxyGroupOrder(raw); len(got) != 0 {
			t.Fatalf("got %#v for %q", got, raw)
		}
	}
}

func TestProxyGroupOrderPreservesEscapedNames(t *testing.T) {
	raw := `proxy-groups: [{name: "\u9999\u6e2f, \"自动\""}, {name: 'Work''s Proxy'}]`
	if got := ProxyGroupOrder(raw); !reflect.DeepEqual(got, []string{"香港, \"自动\"", "Work's Proxy"}) {
		t.Fatalf("got %#v", got)
	}
}
