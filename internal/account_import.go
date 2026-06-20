package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	cdpPage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func normalizeCookieMaterial(cookieJSON, cookieString string) (string, string) {
	cookies := parseCapturedCookies(cookieJSON, cookieString)
	if len(cookies) == 0 {
		return strings.TrimSpace(cookieJSON), strings.TrimSpace(cookieString)
	}
	normalizedJSON := strings.TrimSpace(cookieJSON)
	if b, err := json.Marshal(cookies); err == nil {
		normalizedJSON = string(b)
	}
	normalizedHeader := strings.TrimSpace(cookieString)
	if normalizedHeader == "" {
		normalizedHeader = capturedCookiesToHeader(cookies)
	}
	return normalizedJSON, normalizedHeader
}

func bootstrapImportedAccountProfile(account *AccountRecord) (bool, string) {
	if account == nil {
		return false, "empty account"
	}
	if strings.TrimSpace(account.CookieJSON) == "" && strings.TrimSpace(account.CookieString) == "" {
		return false, "empty cookie material"
	}

	unlock := lockAccountChromeProfile(account.ID)
	defer unlock()

	profile := accountChromeProfilePath(account.ID)
	if err := os.RemoveAll(profile); err != nil {
		return false, "reset persistent profile failed: " + err.Error()
	}
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return false, "create persistent profile failed: " + err.Error()
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("lang", "zh-CN,zh,en-US,en"),
		chromedp.WindowSize(1365, 900),
	)
	if Cfg.ChromeExec != "" {
		opts = append(opts, chromedp.ExecPath(Cfg.ChromeExec))
	}
	userAgent := account.UserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	}
	opts = append(opts, chromedp.UserAgent(userAgent))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	localStorageJSON := strings.TrimSpace(account.LocalStorageJSON)
	if localStorageJSON == "" {
		localStorageJSON = "{}"
	}
	var profileResult browserFetchResult
	var refreshedCookies []*network.Cookie
	refreshedLocalStorage := ""
	refreshedUserAgent := ""
	err := chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := network.Headers{
				"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
			}
			return network.SetExtraHTTPHeaders(headers).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return setBrowserCookies(ctx, "https://klingai.com/", account.CookieJSON, account.CookieString)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := cdpPage.Navigate("https://klingai.com/app").Do(ctx)
			return err
		}),
		chromedp.Sleep(5*time.Second),
		chromedp.Evaluate(`(()=>{try{const data=JSON.parse(`+fmt.Sprintf("%q", localStorageJSON)+`||"{}");for(const [k,v] of Object.entries(data))localStorage.setItem(k,String(v));return true}catch(e){return false}})()`, nil),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpPage.Reload().Do(ctx)
		}),
		chromedp.Sleep(5*time.Second),
		chromedp.Evaluate(`(async()=>{const r=await fetch("/api/user/profile_and_features",{credentials:"include"});return {status:r.status,text:await r.text(),href:location.href}})()`, &profileResult),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			refreshedCookies, err = network.GetCookies().WithURLs(klingCookieURLs("https://klingai.com/")).Do(ctx)
			return err
		}),
		chromedp.Evaluate(`JSON.stringify(Object.fromEntries(Object.entries(localStorage)))`, &refreshedLocalStorage),
		chromedp.Evaluate(`navigator.userAgent`, &refreshedUserAgent),
	)
	if err != nil {
		_ = os.RemoveAll(profile)
		return false, "bootstrap persistent profile failed: " + err.Error()
	}
	cookieString := cookiesToString(refreshedCookies)
	cookieJSON := ""
	if len(refreshedCookies) > 0 {
		if b, err := json.Marshal(refreshedCookies); err == nil {
			cookieJSON = string(b)
		}
	}
	if err := AppStore.UpdateAccountSessionSnapshot(account.ID, cookieJSON, cookieString, refreshedLocalStorage, refreshedUserAgent); err != nil {
		LogError("failed to update imported Kling account session snapshot: %v", err)
	}
	if profileResult.Status < http.StatusOK || profileResult.Status >= http.StatusMultipleChoices {
		_ = os.RemoveAll(profile)
		return false, fmt.Sprintf("klingai.com profile returned HTTP %d: %s", profileResult.Status, trimBody([]byte(profileResult.Text)))
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(profileResult.Text), &out); err != nil {
		_ = os.RemoveAll(profile)
		return false, fmt.Sprintf("decode kling profile failed: %v; body=%s", err, trimBody([]byte(profileResult.Text)))
	}
	if !hasKlingUserProfile(out) {
		_ = os.RemoveAll(profile)
		return false, "klingai.com profile did not include authenticated user: " + trimBody([]byte(profileResult.Text))
	}
	_ = AppStore.AddEvent(account.ID, "profile_saved", "persistent chrome profile bootstrapped from imported cookie", map[string]interface{}{"path": profile})
	return true, "klingai.com persistent browser profile is authenticated"
}
