package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/kong"
)

// NewTOMLResolver creates a kong.Resolver that reads values from a TOML file.
func NewTOMLResolver(path string) (kong.Resolver, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path from CLI flag, not user input
	if err != nil {
		return nil, fmt.Errorf("read toml config: %w", err)
	}

	var values map[string]any
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse toml config: %w", err)
	}

	return kong.ResolverFunc(func(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		name := flag.Name
		parts := strings.Split(name, "-")

		val := resolve(values, parts)
		if val == nil {
			return nil, nil
		}

		return val, nil
	}), nil
}

func resolve(m map[string]any, parts []string) any {
	if len(parts) == 0 {
		return nil
	}

	// Try exact match first.
	key := strings.Join(parts, "_")
	if v, ok := m[key]; ok {
		return v
	}

	// Try nested: first part as section key.
	if len(parts) > 1 {
		if section, ok := m[parts[0]]; ok {
			if sm, ok := section.(map[string]any); ok {
				return resolve(sm, parts[1:])
			}
		}
	}

	// Try single key.
	if v, ok := m[parts[0]]; ok && len(parts) == 1 {
		if reflect.TypeOf(v).Kind() != reflect.Map {
			return v
		}
	}

	return nil
}
