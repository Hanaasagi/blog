package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ──────────────────────────────────────────────
// Front-matter parser (TOML +++ delimited)
// ──────────────────────────────────────────────

type FrontMatter struct {
	raw     string            // original text between +++ delimiters
	fields  map[string]string // key → raw TOML value (keeps quotes, brackets)
	ordered []string          // insertion order of keys
}

func parseFrontMatter(content string) (*FrontMatter, string, error) {
	const delim = "+++"
	idx1 := strings.Index(content, delim)
	if idx1 < 0 {
		return nil, content, fmt.Errorf("no opening +++ found")
	}
	rest := content[idx1+len(delim):]
	idx2 := strings.Index(rest, delim)
	if idx2 < 0 {
		return nil, content, fmt.Errorf("no closing +++ found")
	}
	fmText := rest[:idx2]
	body := rest[idx2+len(delim):]

	fm := &FrontMatter{
		raw:    fmText,
		fields: make(map[string]string),
	}

	scanner := bufio.NewScanner(strings.NewReader(fmText))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		fm.fields[key] = val
		fm.ordered = append(fm.ordered, key)
	}
	return fm, body, nil
}

func (fm *FrontMatter) get(key string) string {
	v, ok := fm.fields[key]
	if !ok {
		return ""
	}
	return strings.Trim(v, `"'`)
}

func (fm *FrontMatter) set(key, value string) {
	quoted := fmt.Sprintf(`"%s"`, value)
	if _, exists := fm.fields[key]; !exists {
		fm.ordered = append(fm.ordered, key)
	}
	fm.fields[key] = quoted
}

func (fm *FrontMatter) serialize() string {
	var buf strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(fm.raw))
	replaced := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}
		key := strings.TrimSpace(trimmed[:eqIdx])
		if newVal, ok := fm.fields[key]; ok {
			// Preserve original indentation style
			prefix := line[:strings.Index(line, key)]
			// Detect spacing around =
			eqPart := trimmed[len(key):eqIdx]
			afterEq := ""
			if eqIdx+1 < len(trimmed) && trimmed[eqIdx+1] == ' ' {
				afterEq = " "
			}
			buf.WriteString(fmt.Sprintf("%s%s%s=%s%s\n", prefix, key, eqPart, afterEq, newVal))
			replaced[key] = true
		} else {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

func rebuildFile(fm *FrontMatter, body string) string {
	return "+++\n" + fm.serialize() + "+++\n" + body
}

// ──────────────────────────────────────────────
// Markdown body → plain text (for LLM context)
// ──────────────────────────────────────────────

var (
	reCodeBlock = regexp.MustCompile("(?s)```[^`]*```")
	reHTMLTag   = regexp.MustCompile("<[^>]+>")
	reLink      = regexp.MustCompile(`\[([^\]]*)\]\([^\)]*\)`)
	reImage     = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]*\)`)
	reHeading   = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBold      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic    = regexp.MustCompile(`\*([^*]+)\*`)
	reMultiNL   = regexp.MustCompile(`\n{3,}`)
)

func mdToPlain(md string) string {
	text := reCodeBlock.ReplaceAllString(md, " [code] ")
	text = reImage.ReplaceAllString(text, "$1")
	text = reLink.ReplaceAllString(text, "$1")
	text = reHTMLTag.ReplaceAllString(text, "")
	text = reHeading.ReplaceAllString(text, "")
	text = reBold.ReplaceAllString(text, "$1")
	text = reItalic.ReplaceAllString(text, "$1")
	text = reMultiNL.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func truncate(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}

// ──────────────────────────────────────────────
// DeepSeek / OpenAI-compatible API client
// ──────────────────────────────────────────────

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type ChatChoice struct {
	Message ChatMessage `json:"message"`
}

type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

type LLMClient struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func newLLMClient(apiKey, baseURL, model string) *LLMClient {
	return &LLMClient{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *LLMClient) chat(ctx context.Context, messages []ChatMessage) (string, error) {
	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// ──────────────────────────────────────────────
// SEO generation logic
// ──────────────────────────────────────────────

type SEOResult struct {
	Description string `json:"description"`
	Summary     string `json:"summary"`
}

const systemPrompt = `You are an SEO specialist for a technical blog.
Given a blog post title and content, generate:
1. "description": A concise meta description for search engines (120-160 characters, in the same language as the article). It should accurately summarize the core topic and attract clicks from search results.
2. "summary": A brief summary for article listing pages (50-100 characters, in the same language as the article). It should capture the essence of the post in a single phrase.

Rules:
- Use the SAME language as the article (if Chinese, write in Chinese; if English, write in English)
- Do NOT include quotes or special characters that would break TOML front matter
- Do NOT include the blog name or author
- Be specific and factual, not generic
- For "description": include the main topic, key technology, and what the reader will learn
- For "summary": a short, punchy phrase that works as a subtitle

Respond in JSON format only: {"description": "...", "summary": "..."}`

func generateSEO(ctx context.Context, client *LLMClient, title, content string) (*SEOResult, error) {
	plain := mdToPlain(content)
	// Limit context to ~3000 chars to save tokens
	plain = truncate(plain, 3000)

	userMsg := fmt.Sprintf("Title: %s\n\nContent:\n%s", title, plain)

	reply, err := client.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	})
	if err != nil {
		return nil, err
	}

	// Strip markdown code fence if present
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	var result SEOResult
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w (raw: %s)", err, reply)
	}

	// Sanitize: remove double quotes that would break TOML
	result.Description = strings.ReplaceAll(result.Description, `"`, "'")
	result.Summary = strings.ReplaceAll(result.Summary, `"`, "'")

	return &result, nil
}

// ──────────────────────────────────────────────
// File processing
// ──────────────────────────────────────────────

func needsSEO(fm *FrontMatter) bool {
	desc := fm.get("description")
	summary := fm.get("summary")
	return desc == "" || summary == ""
}

func processFile(ctx context.Context, client *LLMClient, path string, dryRun bool) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	fm, body, err := parseFrontMatter(string(raw))
	if err != nil {
		return false, fmt.Errorf("parse front matter %s: %w", path, err)
	}

	if !needsSEO(fm) {
		return false, nil
	}

	title := fm.get("title")
	if title == "" {
		return false, fmt.Errorf("no title in %s", path)
	}

	log.Printf("🔄 Generating SEO for: %s", title)
	result, err := generateSEO(ctx, client, title, body)
	if err != nil {
		return false, fmt.Errorf("generate SEO for %s: %w", path, err)
	}

	log.Printf("   description: %s", result.Description)
	log.Printf("   summary:     %s", result.Summary)

	if dryRun {
		log.Printf("   [dry-run] would update %s", path)
		return true, nil
	}

	if fm.get("description") == "" {
		fm.set("description", result.Description)
	}
	if fm.get("summary") == "" {
		fm.set("summary", result.Summary)
	}

	newContent := rebuildFile(fm, body)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}

	log.Printf("   ✅ Updated %s", path)
	return true, nil
}

// ──────────────────────────────────────────────
// Main
// ──────────────────────────────────────────────

func main() {
	var (
		dryRun  bool
		apiKey  string
		baseURL string
		model   string
		all     bool
	)

	flag.BoolVar(&dryRun, "dry-run", false, "preview changes without writing files")
	flag.StringVar(&apiKey, "api-key", "", "DeepSeek API key (or set DEEPSEEK_API_KEY env)")
	flag.StringVar(&baseURL, "base-url", "https://api.deepseek.com/v1", "OpenAI-compatible API base URL")
	flag.StringVar(&model, "model", "deepseek-flash", "model name")
	flag.BoolVar(&all, "all", false, "process all posts (default: only staged/changed files)")
	flag.Parse()

	if apiKey == "" {
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("❌ API key required: use -api-key flag or set DEEPSEEK_API_KEY environment variable")
	}

	client := newLLMClient(apiKey, baseURL, model)
	ctx := context.Background()

	var files []string

	if all {
		// Process all posts
		matches, err := filepath.Glob("content/posts/*/index.md")
		if err != nil {
			log.Fatalf("glob: %v", err)
		}
		files = matches
	} else if flag.NArg() > 0 {
		// Process specific files passed as arguments
		files = flag.Args()
	} else {
		// Default: detect from git staged files (pre-commit mode)
		files = getStagedMarkdownFiles()
	}

	if len(files) == 0 {
		log.Println("ℹ️  No markdown files to process")
		return
	}

	updated := 0
	errors := 0
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		changed, err := processFile(ctx, client, f, dryRun)
		if err != nil {
			log.Printf("⚠️  Error processing %s: %v", f, err)
			errors++
			continue
		}
		if changed {
			updated++
		}
	}

	log.Printf("\n📊 Done: %d files processed, %d updated, %d errors", len(files), updated, errors)

	if !dryRun && updated > 0 {
		// Re-stage updated files so the commit includes the changes
		log.Println("📎 Re-staging updated files...")
		for _, f := range files {
			restageFile(f)
		}
	}
}

func getStagedMarkdownFiles() []string {
	out, err := execCmd("git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
	if err != nil {
		log.Printf("⚠️  Could not get staged files: %v", err)
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".md") && strings.Contains(line, "content/posts/") {
			files = append(files, line)
		}
	}
	return files
}

func restageFile(path string) {
	_, err := execCmd("git", "add", path)
	if err != nil {
		log.Printf("⚠️  Could not re-stage %s: %v", path, err)
	}
}

func execCmd(name string, args ...string) (string, error) {
	cmd := osexec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}
