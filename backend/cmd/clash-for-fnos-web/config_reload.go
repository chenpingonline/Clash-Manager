package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
	"gopkg.in/yaml.v3"
)

func configListeners(raw []byte) ([]map[string]any, error) {
	var config struct {
		Listeners []map[string]any `yaml:"listeners"`
	}
	err := yaml.Unmarshal(raw, &config)
	return config.Listeners, err
}

// Mihomo creates new inbound names before dropping obsolete names. Release
// replaced listeners in a separate reload so renames and port swaps cannot bind
// against the old listeners. Keep unchanged listeners and the old routing/TUN
// configuration alive until the final candidate is applied.
func listenerReleaseConfig(previous, next []byte) ([]byte, error) {
	old, err := configListeners(previous)
	if err != nil {
		return nil, err
	}
	incoming, err := configListeners(next)
	if err != nil {
		return nil, err
	}
	if len(old) == 0 || len(incoming) == 0 {
		return nil, nil
	}
	kept := []map[string]any{}
	for _, prior := range old {
		for _, candidate := range incoming {
			if reflect.DeepEqual(prior, candidate) {
				kept = append(kept, prior)
				break
			}
		}
	}
	// Only additions or only removals can be applied in one reload.
	if len(kept) == len(old) || len(kept) == len(incoming) {
		return nil, nil
	}
	return configyaml.MergeOverrides(previous, map[string]any{"listeners": kept})
}

func putRuntimeConfig(ctx context.Context, client *mihomo.Client, raw []byte) error {
	body, err := json.Marshal(map[string]any{"path": "", "payload": string(raw)})
	if err != nil {
		return err
	}
	return mihomoRequest(ctx, client, http.MethodPut, "/configs?force=true", bytes.NewReader(body), 120*time.Second)
}

func restoreConfigListeners(ctx context.Context, client *mihomo.Client, raw []byte) error {
	normalized, err := configyaml.NormalizeTunDNSHijack(raw)
	if err != nil {
		return err
	}
	raw = normalized
	if _, err := configyaml.TunDNSRequirements(raw); err != nil {
		return err
	}
	listeners, err := configListeners(raw)
	if err != nil {
		return err
	}
	if len(listeners) > 0 {
		cleared, err := configyaml.MergeOverrides(raw, map[string]any{"listeners": []any{}})
		if err != nil {
			return err
		}
		if err := putRuntimeConfig(ctx, client, cleared); err != nil {
			return err
		}
	}
	return putRuntimeConfig(ctx, client, raw)
}

func reloadConfigListeners(ctx context.Context, client *mihomo.Client, raw []byte, previous ...[]byte) error {
	var release []byte
	var err error
	if len(previous) > 0 && len(previous[0]) > 0 {
		release, err = listenerReleaseConfig(previous[0], raw)
		if err != nil {
			return err
		}
	}
	if len(release) == 0 {
		return putRuntimeConfig(ctx, client, raw)
	}
	// Pin credentials through both phases and recovery.
	client, err = client.Snapshot()
	if err != nil {
		return err
	}
	err = putRuntimeConfig(ctx, client, release)
	if err == nil {
		err = putRuntimeConfig(ctx, client, raw)
	}
	if err == nil {
		return nil
	}
	recovery, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if rollbackErr := restoreConfigListeners(recovery, client, previous[0]); rollbackErr != nil {
		return errors.Join(err, fmt.Errorf("恢复原独立代理入口失败: %w", rollbackErr))
	}
	return err
}
