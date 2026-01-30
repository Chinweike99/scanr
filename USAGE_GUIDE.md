# Scanr - AI Code Review Tool - Complete Usage Guide

## Table of Contents
1. [Installation](#installation)
2. [Setup & Configuration](#setup--configuration)
3. [API Key Configuration](#api-key-configuration)
4. [Running Scanr](#running-scanr)
5. [Command Reference](#command-reference)
6. [Examples](#examples)
7. [Configuration File](#configuration-file)
8. [Troubleshooting](#troubleshooting)

---

## Installation

### Prerequisites
- Go 1.18 or higher
- Git
- An API key from one of the supported AI providers

### Step 1: Clone the Repository
```bash
git clone https://github.com/Chinweike99/scanr.git
cd scanr
```

### Step 2: Build the Binary
```bash
go build -o scanr ./cmd/scanr
```

### Step 3: Verify Installation
```bash
./scanr --version
```

### Step 4 (Optional): Add to PATH
```bash
# Copy to a directory in your PATH
sudo cp scanr /usr/local/bin/

# Now you can run scanr from anywhere
scanr --help
```

---

## Setup & Configuration

### Initial Setup

Scanr requires an AI provider API key to function. On first run without configuration, it will prompt you or use environment variables.

#### Option 1: Create Configuration File (Recommended)
```bash
# This creates ~/.scanr-ai.yaml with default settings
scanr init
```

#### Option 2: Set Environment Variable
```bash
# Set the API key as an environment variable
export SCANR_AI_API_KEY="your-api-key-here"
```

---

## API Key Configuration

### Supported AI Providers

#### 1. Google Gemini
**Best for**: Free tier, good performance

```bash
# Get your API key at: https://makersuite.google.com/app/apikey

# Set the API key
export SCANR_AI_API_KEY="your-gemini-api-key"
```

#### 2. OpenAI
**Best for**: GPT-4 quality, most features

```bash
# Get your API key at: https://platform.openai.com/api-keys

# Set the API key
export SCANR_AI_API_KEY="your-openai-api-key"
```

#### 3. Anthropic (Claude)
**Best for**: Advanced reasoning, long context

```bash
# Get your API key at: https://console.anthropic.com/

# Set the API key
export SCANR_AI_API_KEY="your-anthropic-api-key"
```

### Configuration File Setup

#### Step 1: Create/Edit Configuration
```bash
# Edit the configuration file
nano ~/.scanr-ai.yaml
```

#### Step 2: Add Your Configuration

**For Gemini:**
```yaml
provider: gemini
model: gemini-2.5-pro
api_key: your-gemini-api-key-here
max_tokens: 4096
temperature: 0.1
timeout: 60s
max_retries: 3
rate_limit:
  requests_per_minute: 10
  burst: 2
  wait_time: 5s
```

**For OpenAI:**
```yaml
provider: openai
model: gpt-4
api_key: your-openai-api-key-here
base_url: https://api.openai.com/v1
max_tokens: 4096
temperature: 0.1
timeout: 60s
max_retries: 3
rate_limit:
  requests_per_minute: 60
  burst: 5
  wait_time: 1s
```

**For Anthropic:**
```yaml
provider: anthropic
model: claude-3-sonnet-20240229
api_key: your-anthropic-api-key-here
max_tokens: 4096
temperature: 0.1
timeout: 60s
max_retries: 3
rate_limit:
  requests_per_minute: 50
  burst: 3
  wait_time: 2s
```

#### Step 3: Set Secure Permissions
```bash
# Restrict file access (protect your API key)
chmod 600 ~/.scanr-ai.yaml
```

---

## Running Scanr

### Basic Commands

#### 1. Review Changed Files (Git Repository)
```bash
# Review all changed files in git
scanr review

# Output: Shows code issues found in changed files
```

#### 2. Review Only Staged Changes
```bash
# Review only staged files (before commit)
scanr review --staged

# Perfect for pre-commit hooks
```

#### 3. Review Specific Languages
```bash
# Review only Go and Python files
scanr review --languages go,python

# Review only JavaScript/TypeScript
scanr review --languages javascript,typescript
```

#### 4. Set Output Format
```bash
# JSON output (for CI/CD pipelines)
scanr review --format json

# Text output (human-readable)
scanr review --format text
```

#### 5. Limit Number of Files
```bash
# Review only first 10 files
scanr review --max-files 10
```

#### 6. Combine Options
```bash
# Review staged Python files only, show as JSON
scanr review --staged --languages python --format json
```

### Full Command Syntax
```bash
scanr review [options]

Options:
  --staged              Review only staged files (git)
  --languages string    Comma-separated list (go,python,javascript,etc.)
  --format string       Output format: json or text (default: text)
  --max-files int       Maximum files to review (0 = unlimited)
  --help                Show help message
```

---

## Command Reference

### Initialize Configuration
```bash
scanr init
```
Creates a default configuration file at `~/.scanr-ai.yaml`

### Review Code
```bash
scanr review [flags]
```
Runs code review on files in git repository or scans directory

### Available Flags
- `-s, --staged` - Review only staged changes
- `-l, --languages` - Specific languages to review
- `-f, --format` - Output format (json/text)
- `-m, --max-files` - Max files to process
- `-h, --help` - Help information

### Exit Codes
- `0` - Success (info level issues only)
- `1` - Warning level issues found
- `2` - Critical issues found

---

## Examples

### Example 1: Review Staged Files Before Commit
```bash
# Stage your changes
git add .

# Run scanr on staged files
scanr review --staged --format text

# If no critical issues, commit
git commit -m "Add new feature"
```

### Example 2: CI/CD Pipeline Integration
```bash
# In your GitHub Actions workflow
- name: Run Scanr Code Review
  run: |
    scanr review --format json > review-results.json
    
- name: Upload Results
  uses: actions/upload-artifact@v2
  with:
    name: code-review-results
    path: review-results.json
```

### Example 3: Review Specific Language
```bash
# Review only Python files in changed code
scanr review --languages python --format text
```

### Example 4: Pre-commit Hook Setup
```bash
# Create .git/hooks/pre-commit
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
echo "Running Scanr code review..."
scanr review --staged --format text

if [ $? -eq 2 ]; then
  echo "Critical issues found. Commit aborted."
  exit 1
fi
EOF

# Make it executable
chmod +x .git/hooks/pre-commit
```

### Example 5: Non-Git Repository (Full Directory Scan)
```bash
# If not in a git repository, scanr scans the entire directory
scanr review --format json
```

---

## Configuration File

### Location
- **Linux/Mac**: `~/.scanr-ai.yaml`
- **Windows**: `%USERPROFILE%\.scanr-ai.yaml`

### Configuration Options

```yaml
# Provider: gemini, openai, anthropic, or mock
provider: gemini

# Model name (varies by provider)
# Gemini: gemini-2.5-pro, gemini-pro
# OpenAI: gpt-4, gpt-3.5-turbo
# Anthropic: claude-3-sonnet-20240229, claude-3-opus-20240229
model: gemini-2.5-pro

# Your API key (keep this secure!)
api_key: your-api-key-here

# Optional: Custom API base URL
base_url: ""

# Maximum tokens in response (higher = more detailed)
max_tokens: 4096

# Temperature (0.0 = deterministic, 1.0 = creative)
# Lower values for code review (0.1 recommended)
temperature: 0.1

# Request timeout duration
timeout: 60s

# Retry attempts for failed requests
max_retries: 3

# Rate limiting configuration
rate_limit:
  # Requests allowed per minute
  requests_per_minute: 10
  
  # Burst capacity (temporary spike)
  burst: 2
  
  # Wait time between retries
  wait_time: 5s
```

### Adjusting Configuration

Edit `~/.scanr-ai.yaml`:
```bash
nano ~/.scanr-ai.yaml
```

Common adjustments:
- Increase `max_tokens` for more thorough reviews
- Adjust `temperature` for consistency/creativity balance
- Modify `rate_limit` if you hit API limits
- Change `model` to use different AI model

---

## Troubleshooting

### Issue 1: "API Key Not Found"
```bash
# Error: No AI API key found, using mock reviewer

# Solution 1: Set environment variable
export SCANR_AI_API_KEY="your-api-key"

# Solution 2: Create configuration file
scanr init
# Then edit ~/.scanr-ai.yaml with your API key
```

### Issue 2: "Configuration Invalid"
```bash
# Error: AI configuration invalid, using mock reviewer

# Solution: Validate your API key
# - Check if it's correct and not expired
# - Verify provider setting matches your API key
# - Test the API key directly with the provider's API
```

### Issue 3: "Rate Limit Exceeded"
```bash
# Error: too many requests (429)

# Solution: Adjust rate limiting in ~/.scanr-ai.yaml
rate_limit:
  requests_per_minute: 5  # Reduce this
  burst: 1
  wait_time: 10s          # Increase this
```

### Issue 4: "No Files Found"
```bash
# Error: No files found to review

# Solution: Ensure you're in a git repository or changed files exist
git status  # Check git status
ls -la      # Check directory contents
```

### Issue 5: "Provider Not Supported"
```bash
# Error: unsupported AI provider: xyz

# Solution: Use one of these supported providers:
# - gemini
# - openai
# - anthropic
# - mock (for testing)
```

### Issue 6: "Timeout Errors"
```bash
# Error: context deadline exceeded

# Solution: Increase timeout in ~/.scanr-ai.yaml
timeout: 120s  # Increase from 60s

# Or reduce file size limit, or reduce max_tokens
```

### Debug Mode
```bash
# Run with verbose output
SCANR_DEBUG=1 scanr review --format text
```

---

## Best Practices

### 1. Pre-commit Workflow
```bash
# Stage changes
git add .

# Run review on staged files
scanr review --staged

# If good, commit
git commit -m "message"
```

### 2. Cost Management
- Use `--max-files` to limit review scope
- Use `--staged` to review only new changes
- Adjust `max_tokens` down for simple files
- Use `rate_limit` to avoid excessive API calls

### 3. Security
```bash
# Never commit your API key!
chmod 600 ~/.scanr-ai.yaml

# Add to .gitignore
echo "~/.scanr-ai.yaml" >> ~/.gitignore
echo ".scanr-ai.yaml" >> .gitignore
```

### 4. Language Selection
```bash
# Specify languages to avoid unnecessary reviews
scanr review --languages go,python --staged
```

### 5. Batch Processing
```bash
# Review all files, save results
scanr review --format json > results.json

# Process results
cat results.json | jq '.total_issues'
```

---

## CI/CD Integration

### GitHub Actions
```yaml
name: Code Review
on: [pull_request]

jobs:
  scanr:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Set up Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.20
      
      - name: Build Scanr
        run: go build -o scanr ./cmd/scanr
      
      - name: Run Review
        env:
          SCANR_AI_API_KEY: ${{ secrets.SCANR_API_KEY }}
        run: ./scanr review --format json
```

---

## Support & Documentation

- **GitHub**: [Chinweike99/scanr](https://github.com/Chinweike99/scanr)
- **Issues**: Report bugs on GitHub Issues
- **API Documentation**:
  - [Google Gemini](https://ai.google.dev/docs)
  - [OpenAI](https://platform.openai.com/docs)
  - [Anthropic Claude](https://docs.anthropic.com)

---

## License

See LICENSE file in repository.
