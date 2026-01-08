// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/sirupsen/logrus"
)

// Feature flag to enable/disable pipeline annotations
const annotationsFFEnv = "CI_ENABLE_HARNESS_ANNOTATIONS"

// Per-file and per-annotation limits
const (
	maxAnnotationBytes  = 64 * 1024 // 64KB per annotation
	maxAnnotationsCount = 50        // max annotations per file
	defaultPriority     = 3
	postAnnotationsTimeout     = 10 * time.Second
	postAnnotationsMaxRetries  = 2
)

// Mode constants
const (
	modeReplace = "replace"
	modeAppend  = "append"
	modeDelete  = "delete"
)

// isAnnotationsEnabled checks if annotations feature flag is enabled
func isAnnotationsEnabled(envs map[string]string) bool {
	val, ok := envs[annotationsFFEnv]
	if !ok {
		return false
	}
	return strings.ToLower(val) == "true"
}

// annotationsFileRaw represents the raw structure of annotations.json written by hcli
type annotationsFileRaw struct {
	AccountID        string          `json:"accountId,omitempty"`
	OrgID            string          `json:"orgId,omitempty"`
	ProjectID        string          `json:"projectId,omitempty"`
	PipelineID       string          `json:"pipelineId,omitempty"`
	PlanExecutionID  string          `json:"planExecutionId"`
	StageExecutionID string          `json:"stageExecutionId,omitempty"`
	Annotations      []PMSAnnotation `json:"annotations"`
}

// PMSAnnotation is the structure sent to Pipeline Service
type PMSAnnotation struct {
	ContextID string `json:"contextId"`
	Mode      string `json:"mode,omitempty"`
	Style     string `json:"style,omitempty"`	
	Summary   string `json:"summary,omitempty"`	
	Priority  int    `json:"priority,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	StepID    string `json:"stepId,omitempty"`	

}

// createAnnotationsRequest is the payload sent to Pipeline Service
type createAnnotationsRequest struct {
	OrgID            string          `json:"orgId,omitempty"`
	ProjectID        string          `json:"projectId,omitempty"`
	PipelineID       string          `json:"pipelineId,omitempty"`
	StageExecutionID string          `json:"stageExecutionId,omitempty"`
	PlanExecutionID  string          `json:"planExecutionId"`
	Annotations      []PMSAnnotation `json:"annotations"`
}

// pipelineClient is a minimal HTTP client wrapper for posting annotations
type pipelineClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// postAnnotationsToPipeline reads the per-step annotations file and posts annotations directly
// to Pipeline Service. It never fails the step and logs errors on failures.
func (e *StepExecutor) postAnnotationsToPipeline(ctx context.Context, r *api.StartStepRequest) {
	stepID := r.StartStepRequestConfig.ID	
	// Read annotations file
	file := e.readAnnotationsJSON(stepID)
	if file == nil {
		logrus.WithField("step_id", stepID).Warnln("readAnnotationsJSON returned nil")
		return
	}

	// Extract metadata and ensure there are annotations
	planExecutionID := file.PlanExecutionID
	if len(file.Annotations) == 0 || planExecutionID == "" {
		return
	}

	// Fold and sanitize annotations
	annList := foldAnnotationsToSlice(file, stepID)
	if len(annList) == 0 {
		logrus.WithField("step_id", stepID).Warnln("foldAnnotationsToSlice returned empty")
		return
	}	

	accountID := file.AccountID
	if accountID == "" {
		logrus.WithField("step_id", stepID).Warnln("Missing accountId")
		return
	}

	// Build request payload
	payload := createAnnotationsRequest{
		PlanExecutionID:  planExecutionID,
		Annotations:      annList,
		OrgID:            file.OrgID,
		ProjectID:        file.ProjectID,
		PipelineID:       file.PipelineID,
		StageExecutionID: file.StageExecutionID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		logrus.WithError(err).Warnln("ANNOTATIONS: failed to marshal payload")
		return
	}

	// Get annotations credentials from request
	baseURL := r.StartStepRequestConfig.Envs["HARNESS_ANNOTATIONS_SERVICE_ENDPOINT"]
	token := r.StartStepRequestConfig.Envs["HARNESS_ANNOTATIONS_SERVICE_TOKEN"]
	
	if baseURL == "" || token == "" {
		logrus.WithField("step_id", stepID).Warnln("Missing baseURL or token")
		return
	}

	// Build endpoint with query params
	endpoint := fmt.Sprintf("/api/pipelines/annotations?accountId=%s&planExecutionId=%s",
		url.QueryEscape(accountID), url.QueryEscape(planExecutionID))

	// POST to Pipeline Service with retries
	client := newPipelineClient(baseURL, token, postAnnotationsTimeout)
	for attempt := 1; attempt <= postAnnotationsMaxRetries+1; attempt++ {
		_, _, err := client.PostJSON(ctx, endpoint, body)
		if err == nil {
			return
		}
		logrus.WithField("attempt", attempt).WithError(err).Warnln("ANNOTATIONS: post attempt failed")
	}
	logrus.WithField("endpoint", endpoint).Errorln("❌ ANNOTATIONS: post failed after all retries")
}

// readAnnotationsJSON reads and parses the per-step annotations file from the shared volume
func (e *StepExecutor) readAnnotationsJSON(stepID string) *annotationsFileRaw {
	if stepID == "" {
		logrus.Warnln("readAnnotationsJSON: stepID is empty")
		return nil
	}

	path := fmt.Sprintf("%s/%s-annotations.json", pipeline.GetSharedVolPath(), stepID)
	// Convert to OS-specific path (important for Windows)
	osPath := convertToOSPath(path)

	info, err := os.Stat(osPath)
	if err != nil {
		logrus.WithError(err).
			WithField("step_id", stepID).
			WithField("path", osPath).
			Warnln("File stat failed (file may not exist)")
		return nil
	}

	// Cap file size
	maxSize := int64(maxAnnotationsCount*maxAnnotationBytes + 256*1024)
	if info.Size() <= 0 {

		return nil

	}



	if info.Size() > maxSize {

		logrus.WithField("step_id", stepID).WithField("path", path).WithField("size", info.Size()).Warnln("ANNOTATIONS: file too large, skipping")

		return nil

	}

	data, err := os.ReadFile(osPath)
	if err != nil {
		logrus.WithError(err).
			WithField("step_id", stepID).
			Warnln("ANNOTATIONS: read failed")
		return nil
	}

	if !json.Valid(data) {
		logrus.WithField("step_id", stepID).
			WithField("content", string(data[:100])).
			Warnln("ANNOTATIONS: invalid JSON, skipping")
		return nil
	}

	var file annotationsFileRaw
	if err := json.Unmarshal(data, &file); err != nil {
		return nil
	}

	if len(file.Annotations) > maxAnnotationsCount {
		file.Annotations = file.Annotations[:maxAnnotationsCount]
	}

	return &file
}

// foldAnnotationsToSlice folds file annotations by context and applies mode semantics
func foldAnnotationsToSlice(file *annotationsFileRaw, id string) []PMSAnnotation {
	annotations := make(map[string]PMSAnnotation)
	
	for _, a := range file.Annotations {
		
		ctxName := a.ContextID
		if ctxName == "" {
			logrus.WithField("step_id", id).
				Warnln("Skipping - empty context")
			continue
		}
		
		mode := strings.ToLower(a.Mode)
		if mode == "" {
			mode = modeReplace
		}
		
		switch mode {
		case modeAppend, modeReplace, modeDelete:
		default:
			mode = modeReplace
		}

		if mode == modeDelete {
			entry := PMSAnnotation{ContextID: ctxName, Mode: modeDelete}
			if id != "" {
				entry.StepID = id
			}
			annotations[ctxName] = entry
			continue
		}

		// Get or create entry
		entry, ok := annotations[ctxName]
		if !ok {
			entry = PMSAnnotation{ContextID: ctxName, Mode: mode}
		}

		// Update mode (last writer wins)
		entry.Mode = mode

		// Style and priority: last-writer-wins
		if s := strings.ToLower(a.Style); s != "" {
			switch s {
			case "info", "success", "warning", "error":
				entry.Style = s
			default:
				entry.Style = "info"
			}
		}
		
		if a.Priority < 0 || a.Priority > 10 {
			entry.Priority = defaultPriority
		} else {
			entry.Priority = a.Priority
		}

		// Timestamp (optional)
		if a.Timestamp > 0 {
			entry.Timestamp = a.Timestamp
		}

		// Step ID - Use value from file if hcli populated it (from HARNESS_STEP_ID env var)
		// Otherwise fall back to execution ID as last resort
		if a.StepID != "" {
			// Prefer: Step identifier from HARNESS_STEP_ID env var (e.g., "test_styles")
			entry.StepID = a.StepID
		} else if id != "" {
			// Fallback: Use execution ID if file value is empty (e.g., "9q9yFJ8mQRyseno")
			entry.StepID = id
		}

		// Summary (clamped to 64KB)
		if sum := a.Summary; sum != "" {
			if len(sum) > maxAnnotationBytes {
				sum = sum[:maxAnnotationBytes]
			}
			if entry.Summary != "" && mode == modeAppend {
				combined := entry.Summary + "\n" + sum
				if len(combined) > maxAnnotationBytes {
					combined = combined[:maxAnnotationBytes]
				}
				entry.Summary = combined
			} else {
				entry.Summary = sum
			}
		}

		annotations[ctxName] = entry
	}

	if len(annotations) == 0 {
		logrus.WithField("step_id", id).
			Warnln("No annotations to return (map is empty)")
		return nil
	}

	// Convert map to slice
	annList := make([]PMSAnnotation, 0, len(annotations))
	for _, v := range annotations {
		annList = append(annList, v)
	}

	return annList
}

// convertToOSPath converts a Unix-style path to OS-specific format
// This ensures Windows paths work correctly
func convertToOSPath(path string) string {
	return filepath.FromSlash(path)
}

// newPipelineClient creates a new HTTP client for Pipeline Service
func newPipelineClient(baseURL, token string, timeout time.Duration) *pipelineClient {
	return &pipelineClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// PostJSON posts JSON payload to Pipeline Service
func (c *pipelineClient) PostJSON(ctx context.Context, path string, body []byte) (status int, respBody []byte, err error) {
	fullURL := c.baseURL + path
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Annotations %s", c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, _ = io.ReadAll(resp.Body)
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, respBody, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.StatusCode, respBody, nil
}

