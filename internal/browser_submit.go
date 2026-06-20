package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	cdpPage "github.com/chromedp/cdproto/page"
	cdpRuntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type capturedCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

func (c *klingClient) submitWithBrowser(parent context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	return c.requestWithBrowser(parent, "/api/task/submit", "post", payload, nil, "api/task/submit")
}

func (c *klingClient) statusWithBrowser(parent context.Context, upstreamTaskID string) (map[string]interface{}, error) {
	return c.requestWithBrowser(parent, "/api/task/status/batch", "get", nil, map[string]interface{}{"taskIds": upstreamTaskID}, "api/task/status/batch")
}

func (c *klingClient) downloadURLWithBrowser(parent context.Context, workID, taskType string) (map[string]interface{}, error) {
	params := map[string]interface{}{"workIds": workID}
	if taskType == "image" {
		params["fileTypes"] = "PNG"
	}
	return c.requestWithBrowser(parent, "/api/works/batch_download_v2", "get", nil, params, "api/works/batch_download_v2")
}

func (c *klingClient) requestWithBrowser(parent context.Context, apiPath, method string, data, params map[string]interface{}, moduleHint string) (map[string]interface{}, error) {
	unlock := lockAccountChromeProfile(c.account.ID)
	defer unlock()

	profileRoot := filepath.Join(Cfg.DataDir, "chrome-api-profiles")
	if err := os.MkdirAll(profileRoot, 0o755); err != nil {
		return nil, err
	}
	profile := ""
	usesPersistentProfile := accountChromeProfileExists(c.account.ID)
	if usesPersistentProfile {
		profile = accountChromeProfilePath(c.account.ID)
	} else {
		var err error
		profile, _, err = c.prepareSubmitProfile(profileRoot)
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(profile)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(Cfg.ChromeExec),
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees"),
		chromedp.Flag("lang", "zh-CN,zh,en-US,en"),
		chromedp.WindowSize(1365, 900),
		chromedp.UserAgent(c.userAgent),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	runTimeout := 120 * time.Second
	if isSlowSubmitPayload(data) {
		runTimeout = 240 * time.Second
	}
	runCtx, cancelRun := context.WithTimeout(browserCtx, runTimeout)
	defer cancelRun()

	dataJSON, _ := json.Marshal(data)
	if data == nil {
		dataJSON = []byte("null")
	}
	paramsJSON, _ := json.Marshal(params)
	if params == nil {
		paramsJSON = []byte("null")
	}
	localStorage := c.account.LocalStorageJSON
	if strings.TrimSpace(localStorage) == "" {
		localStorage = "{}"
	}
	var result browserFetchResult
	cdpCookieNames := "[]"
	var refreshedCookies []*network.Cookie
	refreshedLocalStorage := ""
	refreshedUserAgent := ""
	actions := []chromedp.Action{
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := network.Headers{
				"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
			}
			return network.SetExtraHTTPHeaders(headers).Do(ctx)
		}),
	}
	if !usesPersistentProfile {
		actions = append(actions,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return setBrowserCookies(ctx, c.baseURL, c.account.CookieJSON, c.account.CookieString)
			}),
		)
	}
	actions = append(actions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := cdpPage.Navigate(c.baseURL + "app").Do(ctx)
			return err
		}),
		chromedp.Sleep(5*time.Second),
	)
	if !usesPersistentProfile {
		actions = append(actions,
			chromedp.Evaluate(`(()=>{const data=JSON.parse(`+strconv.Quote(localStorage)+`||"{}"); for (const [k,v] of Object.entries(data)) localStorage.setItem(k, String(v)); return true;})()`, nil),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpPage.Reload().Do(ctx)
			}),
			chromedp.Sleep(5*time.Second),
		)
	}
	actions = append(actions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().WithURLs(klingCookieURLs(c.baseURL)).Do(ctx)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cookies))
			for _, cookie := range cookies {
				names = append(names, cookie.Name)
			}
			b, _ := json.Marshal(names)
			cdpCookieNames = string(b)
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			result, err = evaluateBrowserFetch(ctx, apiPath, method, string(dataJSON), string(paramsJSON), moduleHint, cdpCookieNames)
			return err
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			refreshedCookies, err = network.GetCookies().WithURLs(klingCookieURLs(c.baseURL)).Do(ctx)
			return err
		}),
		chromedp.Evaluate(`JSON.stringify(Object.fromEntries(Object.entries(localStorage)))`, &refreshedLocalStorage),
		chromedp.Evaluate(`navigator.userAgent`, &refreshedUserAgent),
	)
	err := chromedp.Run(runCtx, actions...)
	if err != nil {
		return nil, err
	}
	cookieString := cookiesToString(refreshedCookies)
	cookieJSON := ""
	if len(refreshedCookies) > 0 {
		if b, err := json.Marshal(refreshedCookies); err == nil {
			cookieJSON = string(b)
		}
	}
	if err := AppStore.UpdateAccountSessionSnapshot(c.account.ID, cookieJSON, cookieString, refreshedLocalStorage, refreshedUserAgent); err != nil {
		LogError("failed to update Kling account session snapshot: %v", err)
	}
	if result.Status < 200 || result.Status >= 300 {
		return nil, fmt.Errorf("Kling browser HTTP %d: %s", result.Status, result.Text)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(result.Text), &out); err != nil {
		return nil, fmt.Errorf("decode Kling browser JSON failed: %w; body=%s", err, result.Text)
	}
	return out, nil
}

func isSlowSubmitPayload(data map[string]interface{}) bool {
	taskType := strings.ToLower(stringFromAny(data["type"]))
	return strings.Contains(taskType, "video") || strings.Contains(taskType, "m2v")
}

func (c *klingClient) prepareSubmitProfile(profileRoot string) (string, bool, error) {
	if source := findReusableLoginProfile(); source != "" {
		target, err := os.MkdirTemp(profileRoot, c.account.ID+"-profile-*")
		if err != nil {
			return "", false, err
		}
		if err := copyChromeProfile(source, target); err != nil {
			_ = os.RemoveAll(target)
			return "", false, err
		}
		return target, true, nil
	}
	profile, err := os.MkdirTemp(profileRoot, c.account.ID+"-*")
	return profile, false, err
}

func findReusableLoginProfile() string {
	root := filepath.Join(Cfg.DataDir, "chrome-profiles")
	cookieDBs, err := filepath.Glob(filepath.Join(root, "*", "Default", "Cookies"))
	if err != nil {
		return ""
	}
	type candidate struct {
		profile string
		mtime   time.Time
	}
	var best candidate
	for _, dbPath := range cookieDBs {
		if !profileCookieDBHasLogin(dbPath) {
			continue
		}
		info, err := os.Stat(dbPath)
		if err != nil {
			continue
		}
		profile := filepath.Dir(filepath.Dir(dbPath))
		if best.profile == "" || info.ModTime().After(best.mtime) {
			best = candidate{profile: profile, mtime: info.ModTime()}
		}
	}
	return best.profile
}

func profileCookieDBHasLogin(dbPath string) bool {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		return false
	}
	defer db.Close()
	var count int
	err = db.QueryRow(`SELECT count(*) FROM cookies WHERE name IN ('kuaishou.ai.portal_st','userId') AND (host_key LIKE '%klingai.com' OR host_key LIKE '%kuaishou%')`).Scan(&count)
	return err == nil && count >= 2
}

func copyChromeProfile(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if name == "SingletonLock" || name == "SingletonCookie" || name == "SingletonSocket" || name == "DevToolsActivePort" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && shouldSkipChromeProfileDir(name) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(dst, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

func shouldSkipChromeProfileDir(name string) bool {
	switch name {
	case "Cache", "Code Cache", "GPUCache", "DawnCache", "ShaderCache", "GrShaderCache", "Crashpad", "BrowserMetrics":
		return true
	default:
		return false
	}
}

func evaluateBrowserFetch(ctx context.Context, apiPath, method, dataJSON, paramsJSON, moduleHint, cdpCookieNames string) (browserFetchResult, error) {
	js := `(async()=>{
		const cdpCookieNames = JSON.parse(` + strconv.Quote(cdpCookieNames) + `);
		try {
			const apiPath = ` + strconv.Quote(apiPath) + `;
			const method = ` + strconv.Quote(strings.ToLower(method)) + `;
			const data = JSON.parse(` + strconv.Quote(dataJSON) + `);
			const params = JSON.parse(` + strconv.Quote(paramsJSON) + `);
			const moduleHint = ` + strconv.Quote(moduleHint) + `;
			const resources = performance.getEntriesByType("resource").map(e=>e.name).filter(name=>name.includes("/assets/js/") && name.endsWith(".js"));
			let moduleURL = "";
			for (const url of resources) {
				try {
					const text = await fetch(url, {credentials: "omit"}).then(r=>r.ok ? r.text() : "");
					if (text.includes("sig4") && text.includes("axiosInstance") && (!moduleHint || text.includes(moduleHint))) {
						moduleURL = url;
						break;
					}
				} catch (_) {}
			}
			if (!moduleURL) {
				for (const url of resources) {
					try {
						const text = await fetch(url, {credentials: "omit"}).then(r=>r.ok ? r.text() : "");
						if (text.includes("sig4") && text.includes("axiosInstance") && text.includes("api/task/submit")) {
							moduleURL = url;
							break;
						}
					} catch (_) {}
				}
			}
			if (!moduleURL) throw new Error("official Kling api module was not found");
			const cookieLength = document.cookie.length;
			const profileText = await fetch("/api/user/profile_and_features", {credentials: "include"}).then(r=>r.text()).catch(e=>String(e));
			const mod = await import(moduleURL);
			const api = mod.a || mod.default;
			if (!api || !api.axiosInstance) throw new Error("official Kling api client export was not found");
			const response = await api.axiosInstance.request({
				url: apiPath,
				method,
				data: data === null ? undefined : data,
				params: params === null ? undefined : params,
				headers: data === null ? undefined : {"Content-Type": "application/json"},
				requestOptions: {}
			});
			return {status: 200, text: JSON.stringify(response && response.data !== undefined ? response.data : response), href: location.href};
		} catch (err) {
			const detail = {
				message: err && err.message,
				stack: err && err.stack,
				status: err && err.status,
				code: err && err.code,
				errorType: err && err.errorType,
				data: err && err.data,
				responseStatus: err && err.response && err.response.status,
				responseData: err && err.response && err.response.data,
				configURL: err && err.config && err.config.url,
				configMethod: err && err.config && err.config.method,
				cdpCookieNames,
				cookieLength: document.cookie.length,
				profileText: await fetch("/api/user/profile_and_features", {credentials: "include"}).then(r=>r.text()).catch(e=>String(e))
			};
			return {status: -1, text: JSON.stringify(detail), href: location.href};
		}
	})()`
	obj, exp, err := cdpRuntime.Evaluate(js).WithAwaitPromise(true).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return browserFetchResult{}, err
	}
	if exp != nil {
		return browserFetchResult{}, errors.New(exp.Text)
	}
	var result browserFetchResult
	if len(obj.Value) == 0 {
		return result, errors.New("browser fetch returned empty CDP value")
	}
	if err := json.Unmarshal(obj.Value, &result); err != nil {
		return result, err
	}
	return result, nil
}

type browserFetchResult struct {
	Status int    `json:"status"`
	Text   string `json:"text"`
	Href   string `json:"href"`
}

func klingCookieURLs(baseURL string) []string {
	urls := []string{
		"https://klingai.com/",
		"https://www.klingai.com/",
		"https://klingai.com/app",
		"https://app.klingai.com/",
		"https://id.klingai.com/",
	}
	if strings.TrimSpace(baseURL) != "" {
		urls = append(urls, baseURL, strings.TrimRight(baseURL, "/")+"/app")
	}
	return urls
}

func setBrowserCookies(ctx context.Context, baseURL, cookieJSON, cookieString string) error {
	cookies := parseCapturedCookies(cookieJSON, cookieString)
	if len(cookies) == 0 {
		return errors.New("no cookies available for browser submit")
	}
	parsed, _ := url.Parse(baseURL)
	for _, item := range cookies {
		if item.Name == "" {
			continue
		}
		domain := item.Domain
		if domain == "" && parsed != nil {
			domain = parsed.Hostname()
		}
		path := item.Path
		if path == "" {
			path = "/"
		}
		cookieURL := browserCookieURL(baseURL, domain)
		action := network.SetCookie(item.Name, item.Value).
			WithURL(cookieURL).
			WithDomain(domain).
			WithPath(path).
			WithHTTPOnly(item.HTTPOnly).
			WithSecure(item.Secure)
		switch strings.ToLower(item.SameSite) {
		case "none":
			action = action.WithSameSite(network.CookieSameSiteNone)
		case "lax":
			action = action.WithSameSite(network.CookieSameSiteLax)
		case "strict":
			action = action.WithSameSite(network.CookieSameSiteStrict)
		}
		if err := action.Do(ctx); err != nil {
			return err
		}
	}
	return nil
}

func parseCapturedCookies(cookieJSON, cookieString string) []capturedCookie {
	var cookies []capturedCookie
	raw := strings.TrimSpace(cookieJSON)
	if raw != "" {
		var value interface{}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err == nil {
			cookies = cookiesFromValue(value)
		}
	}
	if len(cookies) == 0 {
		cookies = cookiesFromHeader(cookieString)
	}
	return normalizeCapturedCookies(cookies)
}

func normalizeCapturedCookies(cookies []capturedCookie) []capturedCookie {
	out := make([]capturedCookie, 0, len(cookies))
	seen := map[string]bool{}
	for _, item := range cookies {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		if item.Domain == "" {
			item.Domain = ".klingai.com"
		}
		if item.Path == "" {
			item.Path = "/"
		}
		key := item.Name + "\x00" + item.Domain + "\x00" + item.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func cookiesFromValue(value interface{}) []capturedCookie {
	switch v := value.(type) {
	case []interface{}:
		cookies := make([]capturedCookie, 0, len(v))
		for _, item := range v {
			if cookie, ok := cookieFromMap(item); ok {
				cookies = append(cookies, cookie)
			}
		}
		return cookies
	case map[string]interface{}:
		if nested, ok := v["cookies"]; ok {
			return cookiesFromValue(nested)
		}
		if cookie, ok := cookieFromMap(v); ok {
			return []capturedCookie{cookie}
		}
		cookies := make([]capturedCookie, 0, len(v))
		for name, raw := range v {
			if name == "" {
				continue
			}
			if stringValue, ok := raw.(string); ok {
				cookies = append(cookies, capturedCookie{Name: name, Value: stringValue, Domain: ".klingai.com", Path: "/"})
			}
		}
		return cookies
	default:
		return nil
	}
}

func cookieFromMap(value interface{}) (capturedCookie, bool) {
	item, ok := value.(map[string]interface{})
	if !ok {
		return capturedCookie{}, false
	}
	name := stringField(item, "name")
	valueText := stringField(item, "value")
	if name == "" {
		return capturedCookie{}, false
	}
	cookie := capturedCookie{
		Name:     name,
		Value:    valueText,
		Domain:   firstStringField(item, "domain", "host", "host_key"),
		Path:     firstStringField(item, "path"),
		Expires:  firstNumberField(item, "expires", "expirationDate", "expiration_date"),
		HTTPOnly: boolField(item, "httpOnly", "http_only", "httponly"),
		Secure:   boolField(item, "secure"),
		SameSite: normalizeSameSite(firstStringField(item, "sameSite", "same_site", "samesite")),
	}
	return cookie, true
}

func capturedCookiesToHeader(cookies []capturedCookie) string {
	parts := make([]string, 0, len(cookies))
	seen := map[string]bool{}
	for _, item := range normalizeCapturedCookies(cookies) {
		if item.Name == "" || seen[item.Name] {
			continue
		}
		seen[item.Name] = true
		parts = append(parts, item.Name+"="+item.Value)
	}
	return strings.Join(parts, "; ")
}

func stringField(item map[string]interface{}, key string) string {
	if value, ok := item[key]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func firstStringField(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringField(item, key); value != "" {
			return value
		}
	}
	return ""
}

func firstNumberField(item map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			switch v := value.(type) {
			case json.Number:
				n, _ := v.Float64()
				return n
			case float64:
				return v
			case int:
				return float64(v)
			case string:
				n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
				return n
			}
		}
	}
	return 0
}

func boolField(item map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			switch v := value.(type) {
			case bool:
				return v
			case string:
				parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
				return parsed
			}
		}
	}
	return false
}

func normalizeSameSite(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "no_restriction", "none":
		return "none"
	case "lax", "strict":
		return value
	default:
		return ""
	}
}

func browserCookieURL(baseURL, domain string) string {
	domain = strings.TrimPrefix(strings.TrimSpace(domain), ".")
	if domain == "" {
		return baseURL
	}
	if strings.Contains(domain, "id.klingai.com") {
		return "https://id.klingai.com/"
	}
	if strings.Contains(domain, "klingai.com") {
		return "https://klingai.com/"
	}
	if strings.Contains(domain, "kwaishou") {
		return "https://" + domain + "/"
	}
	return baseURL
}

func cookiesFromHeader(cookieString string) []capturedCookie {
	parts := strings.Split(cookieString, ";")
	cookies := make([]capturedCookie, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		cookies = append(cookies, capturedCookie{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value), Domain: ".klingai.com", Path: "/"})
	}
	return cookies
}
