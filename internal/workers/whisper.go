package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type WhisperWorker struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewWhisperWorker(baseURL, apiKey string) *WhisperWorker {
	return &WhisperWorker{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (w *WhisperWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "transcribe", Description: "Transcribe audio file to text"},
		{Name: "translate", Description: "Translate audio to English"},
		{Name: "languages", Description: "Get supported languages"},
		{Name: "models", Description: "List available whisper models"},
	}
}

func (w *WhisperWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "transcribe", "whisper_transcribe":
		return w.transcribe(ctx, input)
	case "translate", "whisper_translate":
		return w.translate(ctx, input)
	case "languages", "whisper_languages":
		return w.languages(ctx)
	case "models", "whisper_models":
		return w.listModels(ctx)
	default:
		return nil, nil
	}
}

type TranscribeRequest struct {
	AudioPath string `json:"audio_path"`
	Language  string `json:"language,omitempty"`
	Model     string `json:"model,omitempty"`
}

type TranscriptionResponse struct {
	Text string `json:"text"`
}

func (w *WhisperWorker) transcribe(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req TranscribeRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.AudioPath == "" {
		return nil, fmt.Errorf("audio_path is required")
	}

	// Open the audio file
	file, err := os.Open(req.AudioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %v", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(req.AudioPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}

	if req.Language != "" {
		writer.WriteField("language", req.Language)
	}
	if req.Model != "" {
		writer.WriteField("model", req.Model)
	}
	writer.Close()

	// Use local whisper.cpp server or HuggingFace API
	url := w.baseURL + "/v1/audio/transcriptions"
	if url == "/v1/audio/transcriptions" {
		// Default to HuggingFace Whisper API
		url = "https://api-inference.huggingface.co/models/openai/whisper-large-v3"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if w.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiKey)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper error: %s", string(b))
	}

	var result TranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (w *WhisperWorker) translate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req TranscribeRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.AudioPath == "" {
		return nil, fmt.Errorf("audio_path is required")
	}

	// For translation, we use the translate endpoint
	url := "https://api-inference.huggingface.co/models/openai/whisper-large-v3"

	file, err := os.Open(req.AudioPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(req.AudioPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}

	writer.WriteField("task", "translate")
	if req.Language != "" {
		writer.WriteField("language", req.Language)
	}
	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if w.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiKey)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

func (w *WhisperWorker) languages(ctx context.Context) ([]byte, error) {
	languages := []map[string]string{
		{"code": "en", "name": "English"},
		{"code": "es", "name": "Spanish"},
		{"code": "fr", "name": "French"},
		{"code": "de", "name": "German"},
		{"code": "it", "name": "Italian"},
		{"code": "pt", "name": "Portuguese"},
		{"code": "ru", "name": "Russian"},
		{"code": "zh", "name": "Chinese"},
		{"code": "ja", "name": "Japanese"},
		{"code": "ko", "name": "Korean"},
		{"code": "ar", "name": "Arabic"},
		{"code": "hi", "name": "Hindi"},
		{"code": "nl", "name": "Dutch"},
		{"code": "pl", "name": "Polish"},
		{"code": "tr", "name": "Turkish"},
		{"code": "vi", "name": "Vietnamese"},
		{"code": "th", "name": "Thai"},
		{"code": "id", "name": "Indonesian"},
		{"code": "uk", "name": "Ukrainian"},
		{"code": "cs", "name": "Czech"},
	}
	return json.Marshal(languages)
}

func (w *WhisperWorker) listModels(ctx context.Context) ([]byte, error) {
	models := []map[string]string{
		{"id": "openai/whisper-large-v3", "name": "Whisper Large V3", "languages": "all"},
		{"id": "openai/whisper-large", "name": "Whisper Large", "languages": "all"},
		{"id": "openai/whisper-medium", "name": "Whisper Medium", "languages": "all"},
		{"id": "openai/whisper-small", "name": "Whisper Small", "languages": "all"},
		{"id": "openai/whisper-base", "name": "Whisper Base", "languages": "all"},
		{"id": "openai/whisper-tiny", "name": "Whisper Tiny", "languages": "all"},
		{"id": "distil-whisper/distil-large-v3", "name": "Distil Whisper Large V3", "languages": "all"},
		{"id": "distil-whisper/distil-medium.en", "name": "Distil Whisper Medium (English)", "languages": "en"},
	}
	return json.Marshal(models)
}
