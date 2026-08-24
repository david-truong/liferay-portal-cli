package review

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ChatMessage is one entry in an Ollama /api/chat conversation.
type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a tool invocation the model asked for.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the tool and its arguments.
type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type toolSchema struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  toolParameters `json:"parameters"`
}

type toolParameters struct {
	Type       string                  `json:"type"`
	Properties map[string]toolProperty `json:"properties"`
	Required   []string                `json:"required"`
}

type toolProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

var toolSchemas = []toolSchema{
	{
		Type: "function",
		Function: toolFunction{
			Name:        "git_grep",
			Description: "Run `git grep --cached <pattern> -- <pathspec>` in the checkout to find the established convention for a name or pattern.",
			Parameters: toolParameters{
				Type: "object",
				Properties: map[string]toolProperty{
					"pattern":  {Type: "string"},
					"pathspec": {Type: "string", Description: "Optional pathspec, e.g. '*Exception.java'"},
				},
				Required: []string{"pattern"},
			},
		},
	},
	{
		Type: "function",
		Function: toolFunction{
			Name:        "git_ls_files",
			Description: "Run `git ls-files <glob>` in the checkout to survey existing file naming/location conventions.",
			Parameters: toolParameters{
				Type:       "object",
				Properties: map[string]toolProperty{"glob": {Type: "string"}},
				Required:   []string{"glob"},
			},
		},
	},
	{
		Type: "function",
		Function: toolFunction{
			Name:        "read_file",
			Description: "Read a file (optionally a line range) from the checkout to inspect a precedent.",
			Parameters: toolParameters{
				Type: "object",
				Properties: map[string]toolProperty{
					"path":       {Type: "string"},
					"start_line": {Type: "integer"},
					"end_line":   {Type: "integer"},
				},
				Required: []string{"path"},
			},
		},
	},
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []toolSchema  `json:"tools"`
	Stream   bool          `json:"stream"`
	Options  chatOptions   `json:"options"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
}

type chatResponse struct {
	Message ChatMessage `json:"message"`
	Error   string      `json:"error,omitempty"`
}

func chat(ctx context.Context, host, model string, messages []ChatMessage, tools []toolSchema, timeout time.Duration) (ChatMessage, error) {
	payload := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
		Options:  chatOptions{Temperature: 0, TopP: 1.0},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatMessage{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ChatMessage{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatMessage{}, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatMessage{}, fmt.Errorf("parsing ollama response: %w", err)
	}
	if parsed.Error != "" {
		return ChatMessage{}, fmt.Errorf("ollama: %s", parsed.Error)
	}
	return parsed.Message, nil
}

func runGit(checkoutDir string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = checkoutDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil && ctx.Err() == context.DeadlineExceeded {
		return "(tool error: timed out)"
	}

	out := stdout.String()
	if out == "" {
		out = stderr.String()
	}
	if out == "" {
		out = "(no output)"
	}
	return truncate(out, 4000)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func runTool(checkoutDir, name string, arguments map[string]interface{}) string {
	switch name {
	case "git_grep":
		pattern, _ := arguments["pattern"].(string)
		args := []string{"grep", "--cached", "--line-number", pattern}
		if pathspec, ok := arguments["pathspec"].(string); ok && pathspec != "" {
			args = append(args, "--", pathspec)
		}
		return runGit(checkoutDir, args)

	case "git_ls_files":
		glob, _ := arguments["glob"].(string)
		return runGit(checkoutDir, []string{"ls-files", glob})

	case "read_file":
		return runReadFile(checkoutDir, arguments)

	default:
		return fmt.Sprintf("(unknown tool: %s)", name)
	}
}

func runReadFile(checkoutDir string, arguments map[string]interface{}) string {
	path, _ := arguments["path"].(string)
	fullPath := filepath.Join(checkoutDir, path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("(tool error: %v)", err)
	}
	content := string(data)

	startLine, hasStart := numberField(arguments, "start_line")
	endLine, hasEnd := numberField(arguments, "end_line")
	if !hasStart && !hasEnd {
		return truncate(content, 4000)
	}

	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := 1
	if hasStart && startLine > 0 {
		start = startLine
	}
	end := len(lines)
	if hasEnd && endLine > 0 {
		end = endLine
	}
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}

	if start > end {
		content = ""
	} else {
		content = strings.Join(lines[start-1:end], "")
	}
	return truncate(content, 4000)
}

func numberField(m map[string]interface{}, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
