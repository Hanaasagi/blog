#!/usr/bin/env bash
# Install the SEO auto-generation pre-commit hook
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOK_PATH="$REPO_ROOT/.git/hooks/pre-commit"
TOOL_DIR="$REPO_ROOT/tools/seo-gen"

# Build the tool
echo "🔨 Building seo-gen tool..."
cd "$TOOL_DIR"
go build -o seo-gen .
echo "✅ Built: $TOOL_DIR/seo-gen"

# Create pre-commit hook
cat > "$HOOK_PATH" << 'HOOK'
#!/usr/bin/env bash
# Pre-commit hook: auto-generate SEO description/summary for new/modified blog posts
# Requires DEEPSEEK_API_KEY environment variable

if [ -z "${DEEPSEEK_API_KEY:-}" ]; then
  # Silently skip if no API key is configured
  exit 0
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
TOOL="$REPO_ROOT/tools/seo-gen/seo-gen"

if [ ! -x "$TOOL" ]; then
  echo "⚠️  seo-gen tool not built. Run: cd tools/seo-gen && go build -o seo-gen ."
  exit 0
fi

# Get staged markdown files in content/posts/
STAGED_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep 'content/posts/.*index\.md$' || true)

if [ -z "$STAGED_FILES" ]; then
  exit 0
fi

echo "🤖 SEO auto-gen: checking staged blog posts..."
"$TOOL" $STAGED_FILES

# Re-stage any modified files
for f in $STAGED_FILES; do
  git add "$f"
done
HOOK

chmod +x "$HOOK_PATH"
echo "✅ Pre-commit hook installed: $HOOK_PATH"
echo ""
echo "📝 Usage:"
echo "   1. Set DEEPSEEK_API_KEY in your environment"
echo "   2. Write a blog post with empty description/summary"
echo "   3. git add & git commit — SEO fields will be auto-generated"
echo ""
echo "   Manual usage:"
echo "     ./tools/seo-gen/seo-gen -dry-run content/posts/your-post/index.md"
echo "     ./tools/seo-gen/seo-gen -all -dry-run"
