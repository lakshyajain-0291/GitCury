package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"google.golang.org/grpc/status"
)

var (
	maxRetries int
	retryDelay int
)

func SetTimeoutVar(retries, delay int) {
	maxRetries = retries
	retryDelay = delay
}

func printResponse(resp *genai.GenerateContentResponse) {
	if resp == nil {
		return
	}
	for i, candidate := range resp.Candidates {
		Debug(fmt.Sprintf("[GEMINI]: Candidate %d: %s", i+1, candidate.Content.Parts[0]))
	}
}

func SendToGemini(contextData map[string]string, apiKey string) (string, error) {

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		Error("[GEMINI]: 🚨 Failed to initialize Gemini client: " + err.Error())
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(0.5)
	model.SetMaxOutputTokens(100)
	model.ResponseMIMEType = "application/json"
	model.SystemInstruction = genai.NewUserContent(genai.Text(`
	Generate and return only a commit message as JSON with the key "message".
	Follow these guidelines for the commit message:
	• Capitalize the first word, omit final punctuation. If using conventional commits, use lowercase for the commit type.
	• Use imperative mood in the subject line.
	• Include a commit type (e.g. fix, update, refactor, bump).
	• Limit the first line to ≤ 50 characters, subsequent lines ≤ 72.
	• Be concise and direct; avoid filler words.
	• Do not include newline characters (\n) or similar formatting.

	The commit type can include the following:
	feat – a new feature
	fix – a bug fix
	chore – non-source changes
	refactor – refactored code
	docs – documentation updates
	style – formatting changes
	test – tests
	perf – performance improvements
	ci – continuous integration
	build – build system changes
	revert – revert a previous commit
	`))

	prompt := `file: "` + contextData["file"] + `"\n` +
		`type: "` + contextData["type"] + `",\n\n diff: "` + contextData["diff"] + `"`

	var resp *genai.GenerateContentResponse
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			if strings.Contains(err.Error(), "googleapi: Error 429: You exceeded your current quota, please check your plan and billing details.") || status.Code(err) == 14 { // Retry on specific 429 error or UNAVAILABLE
				Warning(fmt.Sprintf("[GEMINI]: ⚠️ Quota exceeded or service unavailable. Retrying in %d seconds... (attempt %d/%d)", retryDelay, retries+1, maxRetries))
				time.Sleep(time.Duration(retryDelay) * time.Second)
				continue
			}
			return "", err
		}
		break
	}

	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 {
		Error("[GEMINI]: ❌ No content generated by Gemini.")
		return "", errors.New("no content generated by Gemini")
	}

	respMessage := fmt.Sprintf(`%s`, resp.Candidates[0].Content.Parts[0])
	Debug("[GEMINI]: ✨ Response received: " + respMessage)

	var result map[string]string
	err = json.Unmarshal([]byte(respMessage), &result)
	if err != nil {
		Error("[GEMINI]: 🚨 Failed to parse response: " + err.Error())
		return "", err
	}

	message, ok := result["message"]
	if !ok {
		Error("[GEMINI]: ❌ Key 'message' not found in response.")
		return "", errors.New("key 'message' not found in response")
	}

	return message, nil
}
