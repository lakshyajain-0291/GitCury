package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"google.golang.org/grpc/status"
)

func printResponse(resp *genai.GenerateContentResponse) {
	if resp == nil {
		return
	}
	for i, candidate := range resp.Candidates {
		Debug(fmt.Sprintf("Candidate %d: %s", i+1, candidate.Content.Parts[0]))
	}
}

func SendToGemini(contextData map[string]string, apiKey string) (string, error) {
	maxRetries := 5
	retryDelay := 5 // seconds

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		Error("Error creating Gemini client: " + err.Error())
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(0.5)
	model.SetMaxOutputTokens(100)
	model.SystemInstruction = genai.NewUserContent(genai.Text("Generate and return only a commit message for the following diff as json with key - 'message'. Make the message short and specific to this project (root folder). Pay attention to type of file. "))
	model.ResponseMIMEType = "application/json"
	prompt := `file: "` + contextData["file"] + `"\n` + `type: "` + contextData["type"] + `"` + `",\n\n diff: "` + contextData["diff"] + `"`

	var resp *genai.GenerateContentResponse
	for retries := 0; retries < maxRetries; retries++ {
		// Debug(fmt.Sprintf("Generating content for prompt: %s", prompt))
		resp, err = model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			if status.Code(err) == 14 { // 14 corresponds to gRPC status code for UNAVAILABLE
				Error(fmt.Sprintf("Received 429 error, retrying in %d seconds... (attempt %d/%d)", retryDelay, retries+1, maxRetries))
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

	// printResponse(resp)
	if len(resp.Candidates) == 0 {
		Error("No content generated by Gemini")
		return "", errors.New("no content generated by Gemini")
	}

	respMessage := fmt.Sprintf(`%s`, resp.Candidates[0].Content.Parts[0])

	var result map[string]string
	err = json.Unmarshal([]byte(respMessage), &result)
	if err != nil {
		Error("Error unmarshalling response message: " + err.Error())
		return "", err
	}

	message, ok := result["message"]
	if !ok {
		Error("Key 'message' not found in response")
		return "", errors.New("key 'message' not found in response")
	}

	return message, nil

}
