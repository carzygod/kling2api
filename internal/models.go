package internal

import (
	"net/http"
	"time"
)

type modelInfo struct {
	ID              string   `json:"id"`
	Object          string   `json:"object"`
	Created         int64    `json:"created"`
	OwnedBy         string   `json:"owned_by"`
	Capabilities    []string `json:"capabilities"`
	Endpoint        string   `json:"endpoint,omitempty"`
	WebTaskTypes    []string `json:"web_task_types,omitempty"`
	ProviderVersion string   `json:"provider_version,omitempty"`
	SupportStatus   string   `json:"support_status,omitempty"`
	Notes           string   `json:"notes,omitempty"`
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
		{ID: "kling-image", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation"}, Endpoint: "/v1/images/generations", WebTaskTypes: []string{"mmu_txt2img_aiweb"}, ProviderVersion: "1.0", SupportStatus: "mapped"},
		{ID: "kling-image-v1-5", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation", "image_to_image"}, Endpoint: "/v1/images/generations", WebTaskTypes: []string{"mmu_txt2img_aiweb", "mmu_img2img_aiweb"}, ProviderVersion: "1.5", SupportStatus: "mapped"},
		{ID: "kling-image-v2", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation", "image_to_image"}, Endpoint: "/v1/images/generations", WebTaskTypes: []string{"mmu_txt2img_aiweb", "mmu_img2img_aiweb"}, ProviderVersion: "2.0", SupportStatus: "mapped"},
		{ID: "kling-image-v2-1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_generation", "image_to_image"}, Endpoint: "/v1/images/generations", WebTaskTypes: []string{"mmu_txt2img_aiweb", "mmu_img2img_aiweb"}, ProviderVersion: "2.1", SupportStatus: "verified", Notes: "Text-to-image v2.1 has been verified on SH01."},
		{ID: "kling-image-i2i", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_to_image"}, Endpoint: "/v1/images/generations", WebTaskTypes: []string{"mmu_img2img_aiweb"}, ProviderVersion: "2.1", SupportStatus: "mapped", Notes: "Alias for image-to-image; version follows the selected image model default."},
		{ID: "kling-v1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "1.0", SupportStatus: "mapped"},
		{ID: "kling-v1-5", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_img2video"}, ProviderVersion: "1.5", SupportStatus: "mapped"},
		{ID: "kling-v1-6", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "1.6", SupportStatus: "mapped"},
		{ID: "kling-v2-master", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.0", SupportStatus: "mapped"},
		{ID: "kling-video-2.0", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.0", SupportStatus: "mapped", Notes: "Alias for kling-v2-master."},
		{ID: "kling-v2-1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"image_to_video", "first_last_frame_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_img2video"}, ProviderVersion: "2.1", SupportStatus: "mapped"},
		{ID: "kling-v2-1-master", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.1", SupportStatus: "mapped"},
		{ID: "kling-video-2.1", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.1", SupportStatus: "mapped"},
		{ID: "kling-video-2.1-hq", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "high_quality"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video_hq", "m2v_img2video_hq"}, ProviderVersion: "2.1", SupportStatus: "mapped"},
		{ID: "kling-v2-5-turbo", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.5", SupportStatus: "mapped"},
		{ID: "kling-video-2.5-turbo", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.5", SupportStatus: "mapped", Notes: "Alias for kling-v2-5-turbo."},
		{ID: "kling-v2-6", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video", "native_audio"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "2.6", SupportStatus: "mapped"},
		{ID: "kling-v3", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video", "native_audio"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "3.0", SupportStatus: "mapped", Notes: "Kling video 3.0. This is not an image-generation model."},
		{ID: "kling-video-3.0", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"text_to_video", "image_to_video", "first_last_frame_video", "native_audio"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_txt2video", "m2v_img2video"}, ProviderVersion: "3.0", SupportStatus: "mapped", Notes: "Alias for kling-v3."},
		{ID: "kling-video-first-last-frame", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"first_last_frame_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_img2video", "m2v_img2video_hq"}, ProviderVersion: "2.1+", SupportStatus: "mapped"},
		{ID: "kling-action-clone", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"action_clone", "motion_reference_video"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_motion_clone"}, ProviderVersion: "2.1+", SupportStatus: "mapped"},
		{ID: "kling-video-extend", Object: "model", Created: created, OwnedBy: "kling-web", Capabilities: []string{"video_extend"}, Endpoint: "/v1/videos/generations", WebTaskTypes: []string{"m2v_extend_video"}, ProviderVersion: "1.5+", SupportStatus: "mapped"},
	}
}
