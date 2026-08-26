package providerconfig

import (
	"encoding/json"
	"fmt"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func ParseHeaders(specs []string) (map[string]string, error) {
	headers := make(map[string]string, len(specs))
	for index, spec := range specs {
		name, value, found := strings.Cut(spec, "=")
		if !found {
			return nil, fmt.Errorf("opencode header %d must use NAME=VALUE format", index+1)
		}

		name = strings.Trim(name, " \t")
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("opencode header %d has an invalid name", index+1)
		}

		value = strings.Trim(value, " \t")
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("opencode header %d %q has an invalid value", index+1, name)
		}

		headers[textproto.CanonicalMIMEHeaderKey(name)] = value
	}

	return headers, nil
}

func MergeHeaders(data json.RawMessage, headers map[string]string) (json.RawMessage, error) {
	if len(headers) == 0 {
		return data, nil
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("provider config is required when headers are configured")
	}

	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("parse provider config: %w", err)
	}

	providers, ok := content["provider"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("provider must be an object when headers are configured")
	}
	for providerName, rawProvider := range providers {
		provider, ok := rawProvider.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("provider %q must be an object", providerName)
		}

		options, err := objectField(provider, "options", providerName)
		if err != nil {
			return nil, err
		}
		providerHeaders, err := objectField(options, "headers", providerName)
		if err != nil {
			return nil, err
		}

		for existingName := range providerHeaders {
			if _, overridden := headers[textproto.CanonicalMIMEHeaderKey(existingName)]; overridden {
				delete(providerHeaders, existingName)
			}
		}
		for name, value := range headers {
			providerHeaders[name] = value
		}
	}

	merged, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal provider config: %w", err)
	}

	return merged, nil
}

func objectField(parent map[string]any, field, providerName string) (map[string]any, error) {
	raw, exists := parent[field]
	if !exists {
		result := map[string]any{}
		parent[field] = result
		return result, nil
	}

	result, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("provider %q %s must be an object", providerName, field)
	}

	return result, nil
}
