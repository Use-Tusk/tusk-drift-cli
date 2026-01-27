package onboardcloud

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"gopkg.in/yaml.v3"
)

// ConfigUpdater allows callbacks to specify which fields to update in the YAML
type ConfigUpdater struct {
	updates []configUpdate
}

type configUpdate struct {
	path  []string
	value any
}

// Set registers a field to be updated in the YAML file
func (u *ConfigUpdater) Set(path []string, value any) {
	u.updates = append(u.updates, configUpdate{path: path, value: value})
}

// saveToConfig accepts a modifier function that updates the config and declares changes
func saveToConfig(modifier func(*config.Config, *ConfigUpdater) error) error {
	// TODO: consider whether we want to use `findConfigFile` here
	configPath := filepath.Join(".tusk", "config.yaml")

	// Read the raw YAML file
	data, err := os.ReadFile(configPath) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse into yaml.Node - this preserves comments
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updater := &ConfigUpdater{}
	if err := modifier(cfg, updater); err != nil {
		return fmt.Errorf("failed to modify config: %w", err)
	}

	if err := updateYAMLNode(&node, updater.updates); err != nil {
		return fmt.Errorf("failed to update YAML: %w", err)
	}

	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(&node); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	_ = encoder.Close()

	output := []byte(buf.String())

	output = addBlankLinesBetweenSections(output)

	if err := os.WriteFile(configPath, output, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Config has changed, invalidate the cache so that
	// next call to config.Get() will reload the config
	config.Invalidate()

	return nil
}

// addBlankLinesBetweenSections adds blank lines between top-level YAML sections
func addBlankLinesBetweenSections(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var result []string

	for i, line := range lines {
		// Check if current line is a top-level key
		isTopLevel := len(line) > 0 && line[0] != ' ' && line[0] != '#' && line[0] != '\t'

		// If this is a top-level key and we're past the first section,
		// add a blank line before it (if there isn't one already)
		if isTopLevel && i > 0 {
			prevLine := ""
			if len(result) > 0 {
				prevLine = result[len(result)-1]
			}

			// previous line wasn't blank, add a blank line
			if strings.TrimSpace(prevLine) != "" {
				result = append(result, "")
			}
		}

		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n"))
}

// updateYAMLNode updates the yaml.Node tree with the declared updates
func updateYAMLNode(root *yaml.Node, updates []configUpdate) error {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid YAML structure")
	}

	mappingNode := root.Content[0]
	if mappingNode.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node")
	}

	for _, update := range updates {
		if err := updateField(mappingNode, update.path, update.value); err != nil {
			return err
		}
	}

	return nil
}

// updateField finds and updates a field in the yaml.Node tree by path
// If the field doesn't exist, it creates it
func updateField(node *yaml.Node, path []string, value any) error {
	if len(path) == 0 {
		return nil
	}

	// Find the key in the current mapping
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Value == path[0] {
			if len(path) == 1 {
				// Found the target - update its value
				// Clear Tag and Style to prevent explicit tags in output.
				// yaml.v3 outputs explicit tags when Style has TaggedStyle bit set,
				// even if Tag is empty (it infers the tag). Setting Style = 0 prevents this.
				valueNode.Kind = yaml.ScalarNode
				valueNode.Tag = ""
				valueNode.Style = 0
				switch v := value.(type) {
				case bool:
					valueNode.Value = fmt.Sprintf("%t", v)
				case float64:
					valueNode.Value = fmt.Sprintf("%v", v)
				default:
					valueNode.Value = fmt.Sprintf("%v", v)
				}
				return nil
			}

			// Recurse into nested mapping
			if valueNode.Kind == yaml.MappingNode {
				return updateField(valueNode, path[1:], value)
			}
		}
	}

	// Key not found - create it
	if len(path) == 1 {
		// Add key-value pair to current mapping
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: path[0],
		}

		valueNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%v", value),
		}

		node.Content = append(node.Content, keyNode, valueNode)
		return nil
	}

	// Need to create nested structure - create the intermediate mapping
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: path[0],
	}
	nestedMapping := &yaml.Node{
		Kind: yaml.MappingNode,
	}
	node.Content = append(node.Content, keyNode, nestedMapping)

	// Recurse into the newly created mapping
	return updateField(nestedMapping, path[1:], value)
}

func saveServiceIDToConfig(serviceID string) error {
	return saveToConfig(func(cfg *config.Config, u *ConfigUpdater) error {
		cfg.Service.ID = serviceID
		u.Set([]string{"service", "id"}, serviceID)
		return nil
	})
}

func saveRecordingConfig(samplingRate float64, exportSpans, enableEnvVarRecording bool) error {
	return saveToConfig(func(cfg *config.Config, u *ConfigUpdater) error {
		cfg.Recording.SamplingRate = samplingRate
		cfg.Recording.ExportSpans = &exportSpans
		cfg.Recording.EnableEnvVarRecording = &enableEnvVarRecording

		u.Set([]string{"recording", "sampling_rate"}, samplingRate)
		u.Set([]string{"recording", "export_spans"}, exportSpans)
		u.Set([]string{"recording", "enable_env_var_recording"}, enableEnvVarRecording)
		return nil
	})
}
