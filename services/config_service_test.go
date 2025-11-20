package services
import (
	"testing"
)
func TestTemplatesEmbedded(t *testing.T) {
	templates := []string{
		"templates/config.json.template",
		"templates/sscconfig.json.template",
		"templates/motd.txt.template",
	}
	for _, template := range templates {
		data, err := templatesFS.ReadFile(template)
		if err != nil {
			t.Errorf("Failed to read embedded template %s: %v", template, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("Template %s is empty", template)
		} else {
			t.Logf("✅ Template %s embedded successfully (%d bytes)", template, len(data))
		}
	}
}
