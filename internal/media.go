package internal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCameraJSON = `{"type":"empty","horizontal":0,"vertical":0,"zoom":0,"tilt":0,"pan":0,"roll":0}`
	taskTimeout       = 20 * time.Minute
)

type mediaGenerationRequest struct {
	Model           string                 `json:"model"`
	Prompt          string                 `json:"prompt"`
	NegativePrompt  string                 `json:"negative_prompt"`
	N               int                    `json:"n"`
	Count           int                    `json:"count"`
	Size            string                 `json:"size"`
	Resolution      string                 `json:"resolution"`
	AspectRatio     string                 `json:"aspect_ratio"`
	Ratio           string                 `json:"ratio"`
	Image           string                 `json:"image"`
	ImageURL        string                 `json:"image_url"`
	ReferenceImage  string                 `json:"reference_image"`
	FirstFrameImage string                 `json:"first_frame_image"`
	LastFrameImage  string                 `json:"last_frame_image"`
	ActionVideo     string                 `json:"action_video"`
	ActionVideoURL  string                 `json:"action_video_url"`
	MotionVideo     string                 `json:"motion_video"`
	MotionVideoURL  string                 `json:"motion_video_url"`
	VideoURL        string                 `json:"video_url"`
	WorkID          string                 `json:"work_id"`
	Duration        int                    `json:"duration"`
	DurationSeconds int                    `json:"duration_seconds"`
	Seconds         int                    `json:"seconds"`
	Cfg             interface{}            `json:"cfg"`
	Mode            string                 `json:"mode"`
	Sound           string                 `json:"sound"`
	GenerateAudio   *bool                  `json:"generate_audio"`
	CameraJSON      string                 `json:"camera_json"`
	AccountID       string                 `json:"account_id"`
	Async           bool                   `json:"async"`
	Wait            bool                   `json:"wait"`
	Sync            bool                   `json:"sync"`
	Blocking        bool                   `json:"blocking"`
	Payload         map[string]interface{} `json:"payload"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type klingClient struct {
	account    *AccountRecord
	httpClient *http.Client
	baseURL    string
	uploadBase string
	userAgent  string
}

func HandleImageGenerations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	var req mediaGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if req.Model == "" {
		req.Model = "kling-image"
	}
	task, client, payload, err := submitMediaTask(r.Context(), "image", req)
	if err != nil {
		writeError(w, httpStatusForMediaErr(err), mediaErrCode(err), err.Error())
		return
	}
	_ = payload
	if req.Async {
		writeJSON(w, http.StatusOK, taskResponse(task, nil))
		return
	}
	result, err := client.waitForResult(r.Context(), task.UpstreamTaskID, "image", taskTimeout)
	if err != nil {
		_ = AppStore.SetTaskResult(task.ID, "failed", map[string]interface{}{"error": err.Error()}, "upstream_failed", err.Error())
		writeError(w, http.StatusBadGateway, "upstream_failed", err.Error())
		return
	}
	_ = AppStore.SetTaskResult(task.ID, "succeeded", result, "", "")
	writeJSON(w, http.StatusOK, imageResponse(result))
}

func HandleVideoGenerations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	var req mediaGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if req.Model == "" {
		req.Model = "kling-video-2.1"
	}
	task, _, _, err := submitMediaTask(r.Context(), "video", req)
	if err != nil {
		writeError(w, httpStatusForMediaErr(err), mediaErrCode(err), err.Error())
		return
	}
	if req.Wait || req.Sync || req.Blocking {
		respondWithSynchronousTask(w, r, task)
		return
	}
	writeJSON(w, http.StatusOK, taskResponse(task, nil))
}

func HandleVideoGenerationsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	var req mediaGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if req.Model == "" {
		req.Model = "kling-video-2.1"
	}
	task, _, _, err := submitMediaTask(r.Context(), "video", req)
	if err != nil {
		writeError(w, httpStatusForMediaErr(err), mediaErrCode(err), err.Error())
		return
	}
	respondWithSynchronousTask(w, r, task)
}

func HandleVideoTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	id := taskIDFromPath(r.URL.Path)
	respondWithTask(w, r, id)
}

func HandleGenericTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	respondWithTask(w, r, strings.Trim(id, "/"))
}

func respondWithTask(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_task_id", "task id is required")
		return
	}
	task, err := refreshTaskIfNeeded(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task_not_found", "task not found")
			return
		}
		writeError(w, http.StatusBadGateway, "task_refresh_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, taskResponse(task, parseJSONMap(task.ResultJSON)))
}

func respondWithSynchronousTask(w http.ResponseWriter, r *http.Request, task *TaskRecord) {
	account, err := AppStore.GetAccount(task.ProviderAccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_not_found", err.Error())
		return
	}
	client, err := newKlingClient(account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "client_init_failed", err.Error())
		return
	}
	result, err := client.waitForResult(r.Context(), task.UpstreamTaskID, task.Type, taskTimeout)
	if err != nil {
		_ = AppStore.SetTaskResult(task.ID, "failed", map[string]interface{}{"error": err.Error()}, "upstream_failed", err.Error())
		writeError(w, http.StatusBadGateway, "upstream_failed", err.Error())
		return
	}
	_ = AppStore.SetTaskResult(task.ID, "succeeded", result, "", "")
	updated, _ := AppStore.GetTask(task.ID)
	writeJSON(w, http.StatusOK, taskResponse(updated, result))
}

func submitMediaTask(ctx context.Context, taskType string, req mediaGenerationRequest) (*TaskRecord, *klingClient, map[string]interface{}, error) {
	if strings.TrimSpace(req.Prompt) == "" && req.Payload == nil && !isExtendRequest(req) {
		return nil, nil, nil, mediaError{"missing_prompt", "prompt is required unless payload override is provided"}
	}
	account, err := AppStore.SelectRunnableAccount(req.AccountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, mediaError{"no_account", "no enabled Kling account with cookie_string is available"}
		}
		return nil, nil, nil, err
	}
	client, err := newKlingClient(account)
	if err != nil {
		return nil, nil, nil, err
	}
	payload, err := client.buildPayload(ctx, taskType, req)
	if err != nil {
		return nil, nil, nil, err
	}
	task, err := AppStore.CreateTask(TaskRecord{
		Type:              taskType,
		Status:            "queued",
		Model:             req.Model,
		ProviderAccountID: account.ID,
		RequestJSON:       mustJSON(payload),
	})
	if err != nil {
		return nil, nil, nil, err
	}
	upstreamTaskID, response, err := client.submit(ctx, payload)
	if err != nil {
		_ = AppStore.SetTaskResult(task.ID, "failed", map[string]interface{}{"error": err.Error()}, "submit_failed", err.Error())
		return nil, nil, nil, err
	}
	task.UpstreamTaskID = upstreamTaskID
	task.Status = "submitted"
	task.ResponseJSON = mustJSON(response)
	if err := AppStore.SetTaskSubmitted(task.ID, upstreamTaskID, response); err != nil {
		return nil, nil, nil, err
	}
	return task, client, payload, nil
}

func newKlingClient(account *AccountRecord) (*klingClient, error) {
	if account == nil || strings.TrimSpace(account.CookieString) == "" {
		return nil, errors.New("empty Kling cookie string")
	}
	userAgent := account.UserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	}
	transport := &http.Transport{}
	if account.ProxyURL != "" {
		proxyURL, err := url.Parse(account.ProxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	baseURL := "https://klingai.com/"
	uploadBase := "https://upload.uvfuns.com/"
	return &klingClient{
		account:    account,
		httpClient: &http.Client{Timeout: 60 * time.Second, Transport: transport},
		baseURL:    baseURL,
		uploadBase: uploadBase,
		userAgent:  userAgent,
	}, nil
}

func (c *klingClient) buildPayload(ctx context.Context, taskType string, req mediaGenerationRequest) (map[string]interface{}, error) {
	if req.Payload != nil {
		return req.Payload, nil
	}
	if raw, ok := req.Metadata["raw_payload"].(map[string]interface{}); ok {
		return raw, nil
	}
	if taskType == "image" {
		return c.buildImagePayload(ctx, req)
	}
	return c.buildVideoPayload(ctx, req)
}

func (c *klingClient) buildImagePayload(ctx context.Context, req mediaGenerationRequest) (map[string]interface{}, error) {
	count := req.Count
	if count == 0 {
		count = req.N
	}
	if count == 0 {
		count = 1
	}
	if count < 1 {
		count = 1
	}
	if count > 4 {
		count = 4
	}
	ratio := firstNonEmpty(req.AspectRatio, req.Ratio, ratioFromSize(req.Size), "1:1")
	image := firstNonEmpty(req.ImageURL, req.Image, req.ReferenceImage)
	if image != "" {
		imageURL, err := c.prepareMediaURL(ctx, image, "input.png")
		if err != nil {
			return nil, err
		}
		version := kolorsVersion(req.Model)
		args := []map[string]interface{}{
			{"name": "prompt", "value": req.Prompt},
			{"name": "style", "value": "默认"},
			{"name": "aspect_ratio", "value": ratio},
			{"name": "imageCount", "value": strconv.Itoa(count)},
			{"name": "kolors_version", "value": version},
			{"name": "fidelity", "value": "0.5"},
			{"name": "biz", "value": "klingai"},
		}
		if version == "2.0" || version == "2.1" {
			args = append(args, map[string]interface{}{"name": "img_resolution", "value": imageResolution(req)})
		}
		return map[string]interface{}{
			"type": "mmu_img2img_aiweb",
			"inputs": []map[string]interface{}{
				{"inputType": "URL", "url": imageURL, "name": "input"},
			},
			"arguments": args,
		}, nil
	}
	version := kolorsVersion(req.Model)
	args := []map[string]interface{}{
		{"name": "prompt", "value": req.Prompt},
		{"name": "style", "value": "默认"},
		{"name": "aspect_ratio", "value": ratio},
		{"name": "imageCount", "value": strconv.Itoa(count)},
		{"name": "kolors_version", "value": version},
		{"name": "biz", "value": "klingai"},
	}
	if version == "2.0" || version == "2.1" {
		args = append(args, map[string]interface{}{"name": "img_resolution", "value": imageResolution(req)})
	}
	return map[string]interface{}{
		"type":      "mmu_txt2img_aiweb",
		"inputs":    []map[string]interface{}{},
		"arguments": args,
	}, nil
}

func (c *klingClient) buildVideoPayload(ctx context.Context, req mediaGenerationRequest) (map[string]interface{}, error) {
	model := req.Model
	version := klingVersion(model)
	duration := req.Duration
	if duration == 0 {
		duration = req.DurationSeconds
	}
	if duration == 0 {
		duration = req.Seconds
	}
	if duration == 0 {
		duration = 5
	}
	cfg := cfgString(req.Cfg)
	if cfg == "" {
		cfg = "0.5"
	}
	cameraJSON := req.CameraJSON
	if cameraJSON == "" {
		cameraJSON = defaultCameraJSON
	}
	negativePrompt := req.NegativePrompt
	highQuality := strings.Contains(model, "hq") || strings.Contains(model, "pro")
	ratio := firstNonEmpty(req.AspectRatio, req.Ratio, ratioFromSize(req.Size), "16:9")

	firstFrame := firstNonEmpty(req.FirstFrameImage, req.ImageURL, req.Image, req.ReferenceImage)
	lastFrame := req.LastFrameImage
	actionVideo := firstNonEmpty(req.ActionVideoURL, req.MotionVideoURL, req.VideoURL, req.ActionVideo, req.MotionVideo)

	if strings.Contains(model, "extend") || isExtendRequest(req) {
		if req.WorkID == "" && req.VideoURL == "" {
			return nil, mediaError{"missing_video", "work_id or video_url is required for kling-video-extend"}
		}
		input := map[string]interface{}{"name": "input", "inputType": "URL", "url": req.VideoURL}
		if req.WorkID != "" {
			input["fromWorkId"] = req.WorkID
		}
		return map[string]interface{}{
			"type":   "m2v_extend_video",
			"inputs": []map[string]interface{}{input},
			"arguments": []map[string]interface{}{
				{"name": "prompt", "value": req.Prompt},
				{"name": "biz", "value": "klingai"},
			},
		}, nil
	}

	if strings.Contains(model, "action") || strings.Contains(model, "clone") {
		if actionVideo == "" {
			return nil, mediaError{"missing_action_video", "action_video_url, motion_video_url, or video_url is required for kling-action-clone"}
		}
		actionURL, err := c.prepareMediaURL(ctx, actionVideo, "motion.mp4")
		if err != nil {
			return nil, err
		}
		inputs := []map[string]interface{}{
			{"inputType": "URL", "url": actionURL, "name": "motion"},
		}
		if firstFrame != "" {
			firstURL, err := c.prepareMediaURL(ctx, firstFrame, "first.png")
			if err != nil {
				return nil, err
			}
			inputs = append([]map[string]interface{}{{"inputType": "URL", "url": firstURL, "name": "input"}}, inputs...)
		}
		args := []map[string]interface{}{
			{"name": "prompt", "value": req.Prompt},
			{"name": "negative_prompt", "value": negativePrompt},
			{"name": "cfg", "value": cfg},
			{"name": "duration", "value": strconv.Itoa(duration)},
			{"name": "kling_version", "value": version},
			{"name": "camera_json", "value": cameraJSON},
			{"name": "biz", "value": "klingai"},
		}
		args = appendVideoOptionalArgs(args, req)
		return map[string]interface{}{
			"type":      "m2v_motion_clone",
			"inputs":    inputs,
			"arguments": args,
		}, nil
	}

	if lastFrame != "" {
		if firstFrame == "" {
			return nil, mediaError{"missing_first_frame", "first_frame_image or image_url is required when last_frame_image is provided"}
		}
		firstURL, err := c.prepareMediaURL(ctx, firstFrame, "first.png")
		if err != nil {
			return nil, err
		}
		lastURL, err := c.prepareMediaURL(ctx, lastFrame, "last.png")
		if err != nil {
			return nil, err
		}
		modelType := "m2v_img2video"
		if highQuality {
			modelType = "m2v_img2video_hq"
		}
		args := []map[string]interface{}{
			{"name": "prompt", "value": req.Prompt},
			{"name": "negative_prompt", "value": negativePrompt},
			{"name": "cfg", "value": cfg},
			{"name": "duration", "value": strconv.Itoa(duration)},
			{"name": "kling_version", "value": version},
			{"name": "tail_image_enabled", "value": "true"},
			{"name": "camera_json", "value": cameraJSON},
			{"name": "biz", "value": "klingai"},
		}
		args = appendVideoOptionalArgs(args, req)
		return map[string]interface{}{
			"type": modelType,
			"inputs": []map[string]interface{}{
				{"inputType": "URL", "url": firstURL, "name": "input"},
				{"inputType": "URL", "url": lastURL, "name": "tail_image"},
			},
			"arguments": args,
		}, nil
	}

	if firstFrame != "" {
		firstURL, err := c.prepareMediaURL(ctx, firstFrame, "input.png")
		if err != nil {
			return nil, err
		}
		modelType := "m2v_img2video"
		if highQuality {
			modelType = "m2v_img2video_hq"
		}
		args := []map[string]interface{}{
			{"name": "prompt", "value": req.Prompt},
			{"name": "negative_prompt", "value": negativePrompt},
			{"name": "cfg", "value": cfg},
			{"name": "duration", "value": strconv.Itoa(duration)},
			{"name": "kling_version", "value": version},
			{"name": "tail_image_enabled", "value": "false"},
			{"name": "camera_json", "value": cameraJSON},
			{"name": "biz", "value": "klingai"},
		}
		args = appendVideoOptionalArgs(args, req)
		return map[string]interface{}{
			"type": modelType,
			"inputs": []map[string]interface{}{
				{"inputType": "URL", "url": firstURL, "name": "input"},
			},
			"arguments": args,
		}, nil
	}

	modelType := "m2v_txt2video"
	if highQuality {
		modelType = "m2v_txt2video_hq"
	}
	args := []map[string]interface{}{
		{"name": "prompt", "value": req.Prompt},
		{"name": "negative_prompt", "value": negativePrompt},
		{"name": "cfg", "value": cfg},
		{"name": "duration", "value": strconv.Itoa(duration)},
		{"name": "kling_version", "value": version},
		{"name": "aspect_ratio", "value": ratio},
		{"name": "camera_json", "value": cameraJSON},
		{"name": "biz", "value": "klingai"},
	}
	args = appendVideoOptionalArgs(args, req)
	return map[string]interface{}{
		"type":      modelType,
		"inputs":    []map[string]interface{}{},
		"arguments": args,
	}, nil
}

func (c *klingClient) submit(ctx context.Context, payload map[string]interface{}) (string, map[string]interface{}, error) {
	var out map[string]interface{}
	directErr := c.doJSON(ctx, http.MethodPost, c.baseURL+"api/task/submit", payload, &out)
	if directErr == nil {
		if id := taskIDFromSubmitResponse(out); id != "" {
			return id, out, nil
		}
		if browserOut, err := c.submitWithBrowser(ctx, payload); err == nil {
			if id := taskIDFromSubmitResponse(browserOut); id != "" {
				return id, browserOut, nil
			}
			if signedURL := stringFromAny(browserOut["_signed_url"]); signedURL != "" {
				signedOut, err := c.submitSigned(ctx, signedURL, stringFromAny(browserOut["_signed_body"]), payload)
				if err != nil {
					return "", signedOut, err
				}
				if id := taskIDFromSubmitResponse(signedOut); id != "" {
					return id, signedOut, nil
				}
				return "", signedOut, fmt.Errorf("Kling signed response did not include task id: %s", trimBody([]byte(mustJSON(signedOut))))
			}
			return "", browserOut, fmt.Errorf("Kling browser response did not include task id: %s", trimBody([]byte(mustJSON(browserOut))))
		} else {
			return "", out, fmt.Errorf("Kling direct response did not include task id: %s; browser fallback failed: %w", trimBody([]byte(mustJSON(out))), err)
		}
	} else {
		if browserOut, err := c.submitWithBrowser(ctx, payload); err == nil {
			if id := taskIDFromSubmitResponse(browserOut); id != "" {
				return id, browserOut, nil
			}
			if signedURL := stringFromAny(browserOut["_signed_url"]); signedURL != "" {
				signedOut, err := c.submitSigned(ctx, signedURL, stringFromAny(browserOut["_signed_body"]), payload)
				if err != nil {
					return "", signedOut, err
				}
				if id := taskIDFromSubmitResponse(signedOut); id != "" {
					return id, signedOut, nil
				}
				return "", signedOut, fmt.Errorf("Kling signed response did not include task id: %s", trimBody([]byte(mustJSON(signedOut))))
			}
			return "", browserOut, fmt.Errorf("Kling browser response did not include task id: %s", trimBody([]byte(mustJSON(browserOut))))
		} else {
			return "", nil, fmt.Errorf("Kling direct submit failed: %v; browser submit failed: %w", directErr, err)
		}
	}
	return "", out, fmt.Errorf("Kling response did not include task id: %s", trimBody([]byte(mustJSON(out))))
}

func (c *klingClient) submitSigned(ctx context.Context, signedURL, signedBody string, payload map[string]interface{}) (map[string]interface{}, error) {
	endpoint := signedURL
	if strings.HasPrefix(endpoint, "/") {
		endpoint = c.baseURL + strings.TrimPrefix(endpoint, "/")
	}
	var out map[string]interface{}
	var body io.Reader
	if signedBody != "" {
		body = strings.NewReader(signedBody)
	} else {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	if err := c.doJSONWithContent(ctx, http.MethodPost, endpoint, body, "application/json", &out); err != nil {
		return out, err
	}
	return out, nil
}

func taskIDFromSubmitResponse(out map[string]interface{}) string {
	if task, ok := out["task"].(map[string]interface{}); ok {
		if id := stringFromAny(task["id"]); id != "" {
			return id
		}
	}
	if id := stringFromAny(out["taskId"]); id != "" {
		return id
	}
	if data, ok := out["data"].(map[string]interface{}); ok {
		if status := intFromAny(data["status"]); status == 7 {
			return ""
		}
		if task, ok := data["task"].(map[string]interface{}); ok {
			if id := stringFromAny(task["id"]); id != "" {
				return id
			}
		}
		if id := stringFromAny(data["taskId"]); id != "" {
			return id
		}
	}
	return ""
}

func (c *klingClient) waitForResult(ctx context.Context, upstreamTaskID, taskType string, timeout time.Duration) (map[string]interface{}, error) {
	deadline := time.Now().Add(timeout)
	for {
		result, state, err := c.fetchResult(ctx, upstreamTaskID, taskType)
		if err != nil {
			return result, err
		}
		if state == "succeeded" {
			return result, nil
		}
		if state == "failed" {
			return result, errors.New("Kling task failed")
		}
		if time.Now().After(deadline) {
			return result, errors.New("Kling task timeout")
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *klingClient) fetchResult(ctx context.Context, upstreamTaskID, taskType string) (map[string]interface{}, string, error) {
	if upstreamTaskID == "" {
		return nil, "failed", errors.New("empty upstream task id")
	}
	var out map[string]interface{}
	endpoint := c.baseURL + "api/task/status?taskId=" + url.QueryEscape(upstreamTaskID)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, "failed", err
	}
	data, ok := out["data"].(map[string]interface{})
	if !ok {
		return out, "failed", errors.New("Kling status response missing data")
	}
	status := intFromAny(data["status"])
	state := "running"
	if status >= 90 {
		state = "succeeded"
	} else if status == 9 || status == 50 {
		state = "failed"
	}
	urls := []string{}
	if works, ok := data["works"].([]interface{}); ok {
		for _, item := range works {
			work, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			resource := nestedString(work, "resource", "resource")
			if workID := stringFromAny(work["workId"]); workID != "" {
				if cdnURL, err := c.fetchDownloadURL(ctx, workID, taskType); err == nil && cdnURL != "" {
					resource = cdnURL
				}
			}
			if resource != "" {
				urls = append(urls, resource)
			}
		}
	}
	return map[string]interface{}{
		"id":       upstreamTaskID,
		"status":   state,
		"urls":     urls,
		"raw":      out,
		"provider": "kling-web",
	}, state, nil
}

func (c *klingClient) fetchDownloadURL(ctx context.Context, workID, taskType string) (string, error) {
	endpoint := c.baseURL + "api/works/batch_download_v2?workIds=" + url.QueryEscape(workID)
	if taskType == "image" {
		endpoint += "&fileTypes=PNG"
	}
	var out map[string]interface{}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return "", err
	}
	if data, ok := out["data"].(map[string]interface{}); ok {
		return stringFromAny(data["cdnUrl"]), nil
	}
	return "", nil
}

func (c *klingClient) prepareMediaURL(ctx context.Context, value, fallbackName string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if strings.HasPrefix(value, "data:") {
		bytesData, mediaType, err := decodeDataURI(value)
		if err != nil {
			return "", err
		}
		exts, _ := mime.ExtensionsByType(mediaType)
		fileName := fallbackName
		if len(exts) > 0 {
			fileName = strings.TrimSuffix(fallbackName, filepath.Ext(fallbackName)) + exts[0]
		}
		return c.uploadBytes(ctx, fileName, bytesData)
	}
	return "", mediaError{"unsupported_media_input", "media input must be an http(s) URL or data URI"}
}

func (c *klingClient) uploadBytes(ctx context.Context, fileName string, data []byte) (string, error) {
	var tokenOut map[string]interface{}
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"api/upload/issue/token?filename="+url.QueryEscape(fileName), nil, &tokenOut); err != nil {
		return "", err
	}
	token := nestedString(tokenOut, "data", "token")
	if token == "" {
		return "", errors.New("upload token not found")
	}
	var resumeOut map[string]interface{}
	if err := c.doJSON(ctx, http.MethodGet, c.uploadBase+"api/upload/resume?upload_token="+url.QueryEscape(token), nil, &resumeOut); err != nil {
		return "", err
	}
	if intFromAny(resumeOut["result"]) != 1 {
		return "", fmt.Errorf("upload resume failed: %s", mustJSON(resumeOut))
	}
	fragmentURL := c.uploadBase + "api/upload/fragment?upload_token=" + url.QueryEscape(token) + "&fragment_id=0"
	var fragmentOut map[string]interface{}
	if err := c.doJSONWithContent(ctx, http.MethodPost, fragmentURL, bytes.NewReader(data), "application/octet-stream", &fragmentOut); err != nil {
		return "", err
	}
	if intFromAny(fragmentOut["result"]) != 1 {
		return "", fmt.Errorf("upload fragment failed: %s", mustJSON(fragmentOut))
	}
	var completeOut map[string]interface{}
	completeURL := c.uploadBase + "api/upload/complete?upload_token=" + url.QueryEscape(token) + "&fragment_count=1"
	if err := c.doJSON(ctx, http.MethodPost, completeURL, nil, &completeOut); err != nil {
		return "", err
	}
	if intFromAny(completeOut["result"]) != 1 {
		return "", fmt.Errorf("upload complete failed: %s", mustJSON(completeOut))
	}
	var verifyOut map[string]interface{}
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"api/upload/verify/token?token="+url.QueryEscape(token), nil, &verifyOut); err != nil {
		return "", err
	}
	mediaURL := nestedString(verifyOut, "data", "url")
	if mediaURL == "" {
		return "", errors.New("upload verify did not return media url")
	}
	return mediaURL, nil
}

func (c *klingClient) doJSON(ctx context.Context, method, endpoint string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.doJSONWithContent(ctx, method, endpoint, body, "application/json", out)
}

func (c *klingClient) doJSONWithContent(ctx context.Context, method, endpoint string, body io.Reader, contentType string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", c.account.CookieString)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", strings.TrimSuffix(c.baseURL, "/"))
	req.Header.Set("Referer", c.baseURL+"app")
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Kling HTTP %d: %s", resp.StatusCode, trimBody(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode Kling JSON failed: %w; body=%s", err, trimBody(respBody))
	}
	return nil
}

func refreshTaskIfNeeded(ctx context.Context, id string) (*TaskRecord, error) {
	task, err := AppStore.GetTask(id)
	if err != nil {
		return nil, err
	}
	if task.Status == "succeeded" || task.Status == "failed" || task.UpstreamTaskID == "" {
		return task, nil
	}
	account, err := AppStore.GetAccount(task.ProviderAccountID)
	if err != nil {
		return task, nil
	}
	client, err := newKlingClient(account)
	if err != nil {
		return task, nil
	}
	result, state, err := client.fetchResult(ctx, task.UpstreamTaskID, task.Type)
	if err != nil {
		return task, err
	}
	if state == "succeeded" {
		_ = AppStore.SetTaskResult(task.ID, "succeeded", result, "", "")
	} else if state == "failed" {
		_ = AppStore.SetTaskResult(task.ID, "failed", result, "upstream_failed", "Kling task failed")
	} else {
		_ = AppStore.SetTaskResult(task.ID, "running", result, "", "")
	}
	updated, err := AppStore.GetTask(id)
	if err != nil {
		return task, nil
	}
	return updated, nil
}

func imageResponse(result map[string]interface{}) map[string]interface{} {
	urls, _ := result["urls"].([]string)
	data := make([]map[string]string, 0, len(urls))
	for _, item := range urls {
		data = append(data, map[string]string{"url": item})
	}
	return map[string]interface{}{
		"created": time.Now().Unix(),
		"data":    data,
	}
}

func taskResponse(task *TaskRecord, result map[string]interface{}) map[string]interface{} {
	if task == nil {
		return map[string]interface{}{}
	}
	status := task.Status
	if result != nil {
		if s := stringFromAny(result["status"]); s != "" {
			status = s
		}
	}
	urls := []interface{}{}
	if result != nil {
		if typed, ok := result["urls"].([]interface{}); ok {
			urls = typed
		} else if typed, ok := result["urls"].([]string); ok {
			for _, value := range typed {
				urls = append(urls, value)
			}
		}
	}
	data := make([]map[string]string, 0, len(urls))
	for _, item := range urls {
		if value := stringFromAny(item); value != "" {
			data = append(data, map[string]string{"url": value})
		}
	}
	return map[string]interface{}{
		"id":                  task.ID,
		"object":              task.Type + ".generation",
		"status":              status,
		"model":               task.Model,
		"provider":            "KLING-CREATOR-01",
		"provider_task_id":    task.UpstreamTaskID,
		"provider_account_id": task.ProviderAccountID,
		"created_at":          task.CreatedAt,
		"updated_at":          task.UpdatedAt,
		"completed_at":        task.CompletedAt,
		"data":                data,
		"result":              result,
		"error": map[string]string{
			"code":    task.ErrorCode,
			"message": task.ErrorMessage,
		},
	}
}

type mediaError struct {
	code    string
	message string
}

func (e mediaError) Error() string { return e.message }

func mediaErrCode(err error) string {
	var e mediaError
	if errors.As(err, &e) {
		return e.code
	}
	return "upstream_error"
}

func httpStatusForMediaErr(err error) int {
	var e mediaError
	if errors.As(err, &e) {
		switch e.code {
		case "no_account":
			return http.StatusFailedDependency
		case "missing_prompt", "missing_video", "missing_action_video", "missing_first_frame", "unsupported_media_input":
			return http.StatusBadRequest
		}
	}
	return http.StatusBadGateway
}

func taskIDFromPath(path string) string {
	prefixes := []string{"/v1/video/generations/", "/v1/videos/generations/", "/v1/videos/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return strings.Trim(strings.TrimPrefix(path, prefix), "/")
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func klingVersion(model string) string {
	if strings.Contains(model, "v3") || strings.Contains(model, "3.0") {
		return "3.0"
	}
	if strings.Contains(model, "v2-6") || strings.Contains(model, "2.6") {
		return "2.6"
	}
	if strings.Contains(model, "v2-5") || strings.Contains(model, "2.5") {
		return "2.5"
	}
	if strings.Contains(model, "v2-master") || strings.Contains(model, "2.0") {
		return "2.0"
	}
	if strings.Contains(model, "v2-1") {
		return "2.1"
	}
	if strings.Contains(model, "2.1") {
		return "2.1"
	}
	if strings.Contains(model, "v1-6") {
		return "1.6"
	}
	if strings.Contains(model, "1.6") {
		return "1.6"
	}
	if strings.Contains(model, "v1-5") {
		return "1.5"
	}
	if strings.Contains(model, "1.5") {
		return "1.5"
	}
	return "1.0"
}

func kolorsVersion(model string) string {
	model = strings.ToLower(model)
	if strings.Contains(model, "2.1") || strings.Contains(model, "v2-1") || strings.Contains(model, "v2_1") {
		return "2.1"
	}
	if strings.Contains(model, "2.0") || strings.Contains(model, "v2") {
		return "2.0"
	}
	if strings.Contains(model, "1.5") || strings.Contains(model, "v1-5") || strings.Contains(model, "v1_5") {
		return "1.5"
	}
	return "1.0"
}

func imageResolution(req mediaGenerationRequest) string {
	value := strings.ToLower(firstNonEmpty(req.Resolution, req.Size))
	if strings.Contains(value, "2k") || strings.Contains(value, "2048") {
		return "2k"
	}
	if strings.Contains(value, "4k") || strings.Contains(value, "4096") {
		return "4k"
	}
	return "1k"
}

func appendVideoOptionalArgs(args []map[string]interface{}, req mediaGenerationRequest) []map[string]interface{} {
	if req.Mode != "" {
		args = append(args, map[string]interface{}{"name": "mode", "value": req.Mode})
	}
	if req.Sound != "" {
		args = append(args, map[string]interface{}{"name": "sound", "value": req.Sound})
	}
	if req.GenerateAudio != nil {
		value := "false"
		if *req.GenerateAudio {
			value = "true"
		}
		args = append(args, map[string]interface{}{"name": "generate_audio", "value": value})
	}
	return args
}

func cfgString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}

func ratioFromSize(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func decodeDataURI(value string) ([]byte, string, error) {
	header, data, found := strings.Cut(value, ",")
	if !found {
		return nil, "", errors.New("invalid data URI")
	}
	mediaType := strings.TrimPrefix(header, "data:")
	mediaType = strings.TrimSuffix(mediaType, ";base64")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", err
	}
	return decoded, mediaType, nil
}

func parseJSONMap(value string) map[string]interface{} {
	if value == "" {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func intFromAny(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

func stringFromAny(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

func nestedString(root map[string]interface{}, path ...string) string {
	var current interface{} = root
	for _, key := range path {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = m[key]
	}
	return stringFromAny(current)
}

func trimBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

func isExtendRequest(req mediaGenerationRequest) bool {
	return req.WorkID != "" && strings.TrimSpace(req.Prompt) == "" && req.VideoURL != ""
}
