package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func RewriteConfigToSQLite(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config for rewrite: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse config for rewrite: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config root must be a mapping")
	}
	mapping := root.Content[0]
	removeMappingKey(mapping, "endpoints")
	removeMappingKey(mapping, "group")
	storage := ensureMappingValue(mapping, "endpoints_storage")
	setScalarMappingValue(storage, "type", "sqlite")

	encoded, err := encodeYAMLDocument(&root)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".config-migration-*.tmp")
	if err != nil {
		return fmt.Errorf("create config temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("chmod config temp file: %w", err)
	}
	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return fmt.Errorf("write config temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("fsync config temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close config temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace active config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod active config: %w", err)
	}
	return nil
}

func encodeYAMLDocument(root *yaml.Node) ([]byte, error) {
	var buffer yamlBuffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("encode rewritten config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close config encoder: %w", err)
	}
	return buffer.bytes, nil
}

type yamlBuffer struct{ bytes []byte }

func (b *yamlBuffer) Write(p []byte) (int, error) {
	b.bytes = append(b.bytes, p...)
	return len(p), nil
}

func removeMappingKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func ensureMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			if value.Kind != yaml.MappingNode {
				value.Kind = yaml.MappingNode
				value.Tag = "!!map"
				value.Value = ""
				value.Content = nil
			}
			return value
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

func setScalarMappingValue(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Kind = yaml.ScalarNode
			mapping.Content[i+1].Tag = "!!str"
			mapping.Content[i+1].Value = value
			mapping.Content[i+1].Content = nil
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
