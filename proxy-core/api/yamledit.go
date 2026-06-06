package api

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// errSkipWrite lets an editYAMLFile callback bail out without writing the file
// (e.g. nothing changed, or a precondition failed). editYAMLFile treats it as
// success-but-no-write.
var errSkipWrite = errors.New("skip write")

// Comment-preserving config.yaml editing.
//
// Reading the file into a map[string]any and re-marshalling regenerates the
// document from scratch — comments, blank lines and key order are all lost.
// Editing the yaml.Node tree instead keeps every node we don't touch verbatim
// (yaml.Node carries HeadComment / LineComment / FootComment and preserves key
// order), so dashboard edits no longer strip the comments in config.yaml.

// yamlMap wraps a YAML MappingNode for ergonomic edits.
type yamlMap struct{ n *yaml.Node }

// editYAMLFile loads path as a node tree, hands the root mapping to fn for
// in-place edits, then writes it back. If fn returns an error the file is left
// untouched. An empty/missing-content file is treated as an empty mapping.
func editYAMLFile(path string, fn func(root yamlMap) error) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return err
		}
	}
	root := documentRoot(&doc)
	if err := fn(yamlMap{root}); err != nil {
		if errors.Is(err, errSkipWrite) {
			return nil
		}
		return err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// documentRoot returns the top-level mapping node, initialising an empty
// mapping (and document wrapper) for an empty file.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			doc.Content = []*yaml.Node{m}
			return m
		}
		return doc.Content[0]
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	doc.Kind = yaml.DocumentNode
	doc.Content = []*yaml.Node{m}
	return m
}

// get returns the value node for key, or nil if absent.
func (m yamlMap) get(key string) *yaml.Node {
	for i := 0; i+1 < len(m.n.Content); i += 2 {
		if m.n.Content[i].Value == key {
			return m.n.Content[i+1]
		}
	}
	return nil
}

// scalarString returns the string value of a scalar key, or "" if not a scalar.
func (m yamlMap) scalarString(key string) string {
	if v := m.get(key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// setNode assigns valNode to key, replacing the existing value (the key node
// and its comments are preserved) or appending a new key/value pair.
func (m yamlMap) setNode(key string, valNode *yaml.Node) {
	for i := 0; i+1 < len(m.n.Content); i += 2 {
		if m.n.Content[i].Value == key {
			m.n.Content[i+1] = valNode
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.n.Content = append(m.n.Content, keyNode, valNode)
}

// set encodes a Go value to a node and assigns it to key.
func (m yamlMap) set(key string, v any) {
	m.setNode(key, yamlNodeOf(v))
}

// child returns the nested mapping at key, creating an empty one when absent
// (or when the present value isn't a mapping).
func (m yamlMap) child(key string) yamlMap {
	if v := m.get(key); v != nil && v.Kind == yaml.MappingNode {
		return yamlMap{v}
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.setNode(key, child)
	return yamlMap{child}
}

// seq returns the sequence node at key, creating an empty one when absent.
func (m yamlMap) seq(key string) *yaml.Node {
	if v := m.get(key); v != nil && v.Kind == yaml.SequenceNode {
		return v
	}
	s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	m.setNode(key, s)
	return s
}

// del removes key if present; reports whether it existed.
func (m yamlMap) del(key string) bool {
	for i := 0; i+1 < len(m.n.Content); i += 2 {
		if m.n.Content[i].Value == key {
			m.n.Content = append(m.n.Content[:i], m.n.Content[i+2:]...)
			return true
		}
	}
	return false
}

// yamlNodeOf encodes a Go value into a fresh node (scalar / sequence / mapping).
func yamlNodeOf(v any) *yaml.Node {
	n := &yaml.Node{}
	_ = n.Encode(v)
	return n
}
