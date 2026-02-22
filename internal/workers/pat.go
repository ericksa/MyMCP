package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type PATWorker struct {
	baseURL string // URL of the PAT Core service (e.g., http://host.docker.internal:8010)
}

func NewPATWorker(baseURL string) *PATWorker {
	return &PATWorker{baseURL: strings.TrimSuffix(baseURL, "/")}
}

func (w *PATWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "calendar_list", Description: "List calendar events"},
		{Name: "calendar_create", Description: "Create a calendar event"},
		{Name: "calendar_update", Description: "Update a calendar event"},
		{Name: "calendar_delete", Description: "Delete a calendar event"},
		{Name: "task_list", Description: "List tasks"},
		{Name: "task_create", Description: "Create a task"},
		{Name: "task_complete", Description: "Mark a task complete"},
		{Name: "email_list", Description: "List emails"},
		{Name: "email_send", Description: "Send an email"},
		{Name: "email_classify", Description: "Classify an email"},
	}
}

func (w *PATWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	full := fmt.Sprintf("%s/pat/%s", w.baseURL, name)
	// Most tools are simple GET/POST JSON endpoints – deduce method from name prefix
	method := "GET"
	if strings.HasPrefix(name, "create") || strings.HasPrefix(name, "update") || strings.HasPrefix(name, "delete") || strings.HasPrefix(name, "send") {
		method = "POST"
	}
	req, err := http.NewRequestWithContext(ctx, method, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if method == "POST" && len(input) > 0 {
		req.Body = http.NoBody // placeholder – will be replaced below
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(input)), nil }
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PAT service error %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
