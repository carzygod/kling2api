package internal

import (
	"net/http"
	"time"
)

type modelInfo struct {
	ID           string   `json:"id"`
	Object       string   `json:"object"`
	Created      int64    `json:"created"`
	OwnedBy      string   `json:"owned_by"`
	Capabilities []string `json:"capabilities"`
}

func HandleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	if !requireAPIKey(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   klingModels(),
	})
}

func klingModels() []modelInfo {
	created := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC).Unix()
	return []modelInfo{
		{ID: "kling-image", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation"}},
		{ID: "kling-image-i2i", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation", "image_to_image"}},
		{ID: "kling-image-v2", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation"}},
		{ID: "kling-image-v2-1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation", "image_to_image"}},
		{ID: "kling-v1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-v1-5", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_to_video"}},
		{ID: "kling-v1-6", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-v2-master", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-v2-1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_to_video", "first_last_frame_video"}},
		{ID: "kling-v2-1-master", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video"}},
		{ID: "kling-v2-5-turbo", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video"}},
		{ID: "kling-v2-6", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video", "native_audio"}},
		{ID: "kling-v3", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video", "native_audio"}},
		{ID: "kling-video-1.0", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-video-1.5", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-video-1.6", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-video-2.1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}},
		{ID: "kling-video-2.1-hq", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "high_quality"}},
		{ID: "kling-video-first-last-frame", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"first_last_frame_video"}},
		{ID: "kling-action-clone", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"action_clone", "motion_reference_video"}},
		{ID: "kling-video-extend", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"video_extend"}},
	}
}
