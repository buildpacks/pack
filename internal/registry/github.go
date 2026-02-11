package registry

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

type GithubIssue struct {
	Title string
	Body  string
}

// CreateGithubIssue creates a GitHub issue from a buildpack. If registryRoot is provided,
// it will attempt to load both title and body templates from the registry-index repository,
// falling back to hardcoded templates if the file doesn't exist.
// Returns the issue and a boolean indicating whether templates were loaded from the registry.
func CreateGithubIssue(b Buildpack, registryRoot ...string) (GithubIssue, error) {
	titleTemplateStr := GithubIssueTitleTemplate
	bodyTemplateStr := GithubIssueBodyTemplate

	// Try to load templates from registry if root is provided
	if len(registryRoot) > 0 && registryRoot[0] != "" {
		if title, body, err := loadTemplatesFromRegistry(registryRoot[0], b.Yanked); err == nil {
			titleTemplateStr = title
			bodyTemplateStr = body
		}
		// Silently fall back to hardcoded templates if loading fails
	}

	titleTemplate, err := template.New("buildpack").Parse(titleTemplateStr)
	if err != nil {
		return GithubIssue{}, err
	}

	bodyTemplate, err := template.New("buildpack").Parse(bodyTemplateStr)
	if err != nil {
		return GithubIssue{}, err
	}

	var title bytes.Buffer
	err = titleTemplate.Execute(&title, b)
	if err != nil {
		return GithubIssue{}, err
	}

	var body bytes.Buffer
	err = bodyTemplate.Execute(&body, b)
	if err != nil {
		return GithubIssue{}, err
	}

	return GithubIssue{
		title.String(),
		body.String(),
	}, nil
}

// loadTemplatesFromRegistry loads both title and body templates from the registry-index repository.
// It parses the YAML frontmatter to extract the title and extracts the body from the markdown content.
func loadTemplatesFromRegistry(registryRoot string, yanked bool) (string, string, error) {
	var templatePath string
	if yanked {
		templatePath = ".github/ISSUE_TEMPLATE/yank-buildpack.md"
	} else {
		templatePath = ".github/ISSUE_TEMPLATE/add-buildpack.md"
	}

	fullPath := filepath.Join(registryRoot, templatePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", "", err
	}

	contentStr := string(content)

	// Parse YAML frontmatter to extract title
	titleTemplate, bodyTemplate, err := parseTemplateFile(contentStr)
	if err != nil {
		return "", "", err
	}

	// Convert placeholders to Go template syntax
	titleTemplate = convertPlaceholdersToGoTemplate(titleTemplate)
	bodyTemplate = convertPlaceholdersToGoTemplate(bodyTemplate)

	return titleTemplate, bodyTemplate, nil
}

// parseTemplateFile parses a GitHub issue template file with YAML frontmatter
// and returns the title template and body template.
func parseTemplateFile(content string) (string, string, error) {
	// Split frontmatter from body
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return "", "", errors.New("invalid template format: missing YAML frontmatter")
	}

	frontmatter := parts[1]
	body := parts[2]

	// Extract title from frontmatter
	titleMatch := regexp.MustCompile(`(?m)^title:\s*(.+)$`).FindStringSubmatch(frontmatter)
	if len(titleMatch) < 2 {
		return "", "", errors.New("invalid template format: missing title in frontmatter")
	}
	titleTemplate := strings.TrimSpace(titleMatch[1])
	// Remove quotes if present
	titleTemplate = strings.Trim(titleTemplate, `"'`)

	return titleTemplate, strings.TrimSpace(body), nil
}

// convertPlaceholdersToGoTemplate converts template placeholders like {BUILDPACK_ID} and {VERSION}
// to Go template syntax like {{.Namespace}}/{{.Name}} and {{.Version}}.
func convertPlaceholdersToGoTemplate(templateStr string) string {
	// Replace {BUILDPACK_ID} with {{.Namespace}}/{{.Name}}
	templateStr = strings.ReplaceAll(templateStr, "{BUILDPACK_ID}", "{{.Namespace}}/{{.Name}}")
	// Replace {VERSION} with {{.Version}}
	templateStr = strings.ReplaceAll(templateStr, "{VERSION}", "{{.Version}}")

	return templateStr
}

func CreateBrowserCmd(browserURL, os string) (*exec.Cmd, error) {
	_, err := url.ParseRequestURI(browserURL)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid URL %s", style.Symbol(browserURL))
	}

	switch os {
	case "linux":
		return exec.Command("xdg-open", browserURL), nil
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", browserURL), nil
	case "darwin":
		return exec.Command("open", browserURL), nil
	default:
		return nil, fmt.Errorf("unsupported platform %s", style.Symbol(os))
	}
}

func GetIssueURL(githubURL string) (*url.URL, error) {
	if githubURL == "" {
		return nil, errors.New("missing github URL")
	}
	return url.Parse(fmt.Sprintf("%s/issues/new", strings.TrimSuffix(githubURL, "/")))
}
