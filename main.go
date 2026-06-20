package main

import (
	"fmt"
	"net/http"

	"klingcreator2api/internal"
)

func main() {
	internal.LoadConfig()
	internal.InitLogger()

	if err := internal.InitStore(); err != nil {
		internal.LogError("sqlite init failed: %v", err)
		return
	}
	internal.CleanupTransientChromeProfiles()

	internal.LoginSessions = internal.NewLoginSessionManager()
	internal.StartAccountKeepalive()

	http.HandleFunc("/health", internal.HandleHealth)
	http.HandleFunc("/admin", internal.HandleAdminPage)
	http.HandleFunc("/api/", internal.HandleAdminAPI)
	http.HandleFunc("/v1/models", internal.HandleModels)
	http.HandleFunc("/v1/images/generations", internal.HandleImageGenerations)
	http.HandleFunc("/v1/video/generations/sync", internal.HandleVideoGenerationsSync)
	http.HandleFunc("/v1/videos/generations/sync", internal.HandleVideoGenerationsSync)
	http.HandleFunc("/v1/video/generations", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/video/generations/", internal.HandleVideoTask)
	http.HandleFunc("/v1/videos/generations", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/videos/generations/", internal.HandleVideoTask)
	http.HandleFunc("/v1/videos", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/videos/", internal.HandleVideoTask)
	http.HandleFunc("/v1/tasks/", internal.HandleGenericTask)

	addr := fmt.Sprintf("%s:%s", internal.Cfg.Host, internal.Cfg.Port)
	internal.LogInfo("KLING-CREATOR-01 listening on %s", addr)
	internal.LogInfo("Admin URL: http://%s:%s/admin?key=%s", internal.Cfg.Host, internal.Cfg.Port, internal.Cfg.AdminKey)
	if err := http.ListenAndServe(addr, nil); err != nil {
		internal.LogError("server failed: %v", err)
	}
}
