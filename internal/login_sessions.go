package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	cdpPage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

type LoginSession struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	LoginURL  string `json:"login_url"`

	ctx        context.Context
	cancel     context.CancelFunc
	profile    string
	screenshot []byte
	mu         sync.Mutex
	runMu      sync.Mutex
}

type LoginSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*LoginSession
}

var LoginSessions *LoginSessionManager

func NewLoginSessionManager() *LoginSessionManager {
	return &LoginSessionManager{sessions: map[string]*LoginSession{}}
}

func (m *LoginSessionManager) Create(name string) (*LoginSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	if name == "" {
		name = "kling-login-" + id[:8]
	}
	profile := filepath.Join(Cfg.DataDir, "chrome-profiles", id)
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return nil, err
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1365,900"),
		chromedp.Flag("lang", "zh-CN,zh,en-US,en"),
		chromedp.UserDataDir(profile),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"),
	)
	if Cfg.ChromeExec != "" {
		opts = append(opts, chromedp.ExecPath(Cfg.ChromeExec))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	session := &LoginSession{
		ID:        id,
		Name:      name,
		Status:    "starting",
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
		LoginURL:  Cfg.LoginURL,
		ctx:       ctx,
		cancel: func() {
			cancel()
			allocCancel()
		},
		profile: profile,
	}
	m.sessions[id] = session
	go session.start()
	return session, nil
}

func (m *LoginSessionManager) List() []*LoginSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*LoginSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s.publicCopy())
	}
	return result
}

func (m *LoginSessionManager) Get(id string) (*LoginSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *LoginSessionManager) Delete(id string) bool {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		session.Close()
	}
	return ok
}

func (s *LoginSession) start() {
	s.setStatus("opening")
	err := s.runBrowser(35*time.Second,
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := cdpPage.Navigate(Cfg.LoginURL).Do(ctx)
			return err
		}),
		chromedp.Sleep(6*time.Second),
	)
	if err != nil {
		s.setError(err)
		return
	}
	s.Status = "waiting_scan"
	s.UpdatedAt = nowISO()
	_ = s.RefreshScreenshot()
}

func (s *LoginSession) publicCopy() *LoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *s
	cp.ctx = nil
	cp.cancel = nil
	cp.screenshot = nil
	return &cp
}

func (s *LoginSession) RefreshScreenshot() error {
	var image []byte
	err := s.runBrowser(20*time.Second,
		chromedp.Sleep(500*time.Millisecond),
		chromedp.FullScreenshot(&image, 85),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.screenshot = image
	s.UpdatedAt = nowISO()
	return nil
}

func (s *LoginSession) Screenshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.screenshot))
	copy(out, s.screenshot)
	return out
}

func (s *LoginSession) Click(x, y float64) error {
	err := s.runBrowser(20*time.Second,
		chromedp.MouseClickXY(x, y),
		chromedp.Sleep(800*time.Millisecond),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	return s.RefreshScreenshot()
}

func (s *LoginSession) OpenQRCodeLogin() error {
	err := s.runBrowser(30*time.Second,
		chromedp.KeyEvent("\u001b"),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.MouseClickXY(38, 736),
		chromedp.Sleep(3200*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var clicked bool
			script := `(() => {
				const visible = el => {
					const r = el.getBoundingClientRect();
					return r.width > 0 && r.height > 0;
				};
				const tabs = Array.from(document.querySelectorAll('*'))
					.filter(el => visible(el) && (el.textContent || '').trim() === '扫码登录');
				if (tabs[0]) {
					tabs[0].click();
					return true;
				}
				return false;
			})()`
			if err := chromedp.Evaluate(script, &clicked).Do(ctx); err != nil {
				return err
			}
			if clicked {
				return nil
			}
			return chromedp.MouseClickXY(904, 342).Do(ctx)
		}),
		chromedp.Sleep(2500*time.Millisecond),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	return s.RefreshScreenshot()
}

func (s *LoginSession) Input(text string) error {
	err := s.runBrowser(20*time.Second,
		chromedp.KeyEvent(text),
		chromedp.Sleep(800*time.Millisecond),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	return s.RefreshScreenshot()
}

func (s *LoginSession) Reload() error {
	s.setStatus("opening")
	err := s.runBrowser(35*time.Second,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpPage.Reload().Do(ctx)
		}),
		chromedp.Sleep(4*time.Second),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	return s.RefreshScreenshot()
}

func (s *LoginSession) Capture(name string) (*AccountRecord, error) {
	if name == "" {
		name = s.Name
	}
	var cookies []*network.Cookie
	localStorageJSON := "{}"
	userAgent := ""
	err := s.runBrowser(25*time.Second,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs(klingCookieURLs(Cfg.LoginURL)).Do(ctx)
			return err
		}),
		chromedp.Evaluate(`JSON.stringify(Object.fromEntries(Object.entries(localStorage)))`, &localStorageJSON),
		chromedp.Evaluate(`navigator.userAgent`, &userAgent),
	)
	if err != nil {
		s.setError(err)
		return nil, err
	}
	cookieString := cookiesToString(cookies)
	if cookieString == "" {
		return nil, errors.New("no cookies captured from klingai.com")
	}
	cookieJSON, _ := json.Marshal(cookies)
	probe := &AccountRecord{
		Name:             name,
		CookieJSON:       string(cookieJSON),
		CookieString:     cookieString,
		LocalStorageJSON: localStorageJSON,
		UserAgent:        userAgent,
	}
	if ok, message := testKlingAccount(probe); !ok {
		return nil, errors.New("captured Kling session is not authenticated: " + message)
	}
	account, err := AppStore.CreateAccount(AccountRecord{
		Name:             name,
		Status:           "valid",
		CookieJSON:       string(cookieJSON),
		CookieString:     cookieString,
		LocalStorageJSON: localStorageJSON,
		UserAgent:        userAgent,
	})
	if err != nil {
		s.setError(err)
		return nil, err
	}
	if err := persistAccountChromeProfile(account.ID, s.profile); err != nil {
		_ = AppStore.DeleteAccount(account.ID)
		s.setError(err)
		return nil, fmt.Errorf("persist Kling browser profile failed: %w", err)
	}
	_ = AppStore.AddEvent(account.ID, "profile_saved", "persistent chrome profile saved", map[string]interface{}{"path": accountChromeProfilePath(account.ID)})
	_ = AppStore.SetAccountTestResult(account.ID, true, "klingai.com profile is authenticated")
	s.mu.Lock()
	s.Status = "captured"
	s.UpdatedAt = nowISO()
	s.mu.Unlock()
	return account, nil
}

func (s *LoginSession) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.profile != "" {
		_ = os.RemoveAll(s.profile)
	}
}

func (s *LoginSession) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setErrorLocked(err)
}

func (s *LoginSession) setErrorLocked(err error) {
	s.Status = "error"
	s.LastError = err.Error()
	s.UpdatedAt = nowISO()
}

func (s *LoginSession) setStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.UpdatedAt = nowISO()
}

func (s *LoginSession) runBrowser(timeout time.Duration, actions ...chromedp.Action) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	_ = timeout
	return chromedp.Run(s.ctx, actions...)
}

func cookiesToString(cookies []*network.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
	}
	return strings.Join(parts, "; ")
}

func testKlingAccount(a *AccountRecord) (bool, string) {
	if a == nil {
		return false, "empty account"
	}
	if a.CookieString == "" && !accountChromeProfileExists(a.ID) {
		return false, "empty cookie string"
	}
	if accountChromeProfileExists(a.ID) {
		client, err := newKlingClient(a)
		if err != nil {
			return false, err.Error()
		}
		out, err := client.requestWithBrowser(context.Background(), "/api/user/profile_and_features", http.MethodGet, nil, nil, "api/user/profile_and_features")
		if err != nil {
			return false, err.Error()
		}
		if hasKlingUserProfile(out) {
			return true, "klingai.com persistent browser profile is authenticated"
		}
		return false, fmt.Sprintf("klingai.com persistent browser profile did not include authenticated user: %s", trimBody([]byte(mustJSON(out))))
	}
	if ok, message := hasFreshTaskCookies(a.CookieJSON); !ok {
		return false, message
	}
	req, err := http.NewRequest(http.MethodGet, "https://klingai.com/api/user/profile_and_features", nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Cookie", a.CookieString)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://klingai.com/app")
	if a.UserAgent != "" {
		req.Header.Set("User-Agent", a.UserAgent)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("klingai.com profile returned HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, fmt.Sprintf("decode kling profile failed: %v; body=%s", err, trimBody(body))
	}
	if hasKlingUserProfile(out) {
		return true, "klingai.com profile is authenticated"
	}
	return false, fmt.Sprintf("klingai.com profile did not include authenticated user: %s", trimBody(body))
}

func hasKlingUserProfile(out map[string]interface{}) bool {
	if out == nil {
		return false
	}
	if profile, ok := out["userProfile"].(map[string]interface{}); ok && len(profile) > 0 {
		return true
	}
	if user, ok := out["user"].(map[string]interface{}); ok && len(user) > 0 {
		return true
	}
	if data, ok := out["data"].(map[string]interface{}); ok {
		if profile, ok := data["userProfile"].(map[string]interface{}); ok && len(profile) > 0 {
			return true
		}
		if user, ok := data["user"].(map[string]interface{}); ok && len(user) > 0 {
			return true
		}
	}
	return false
}

func hasFreshTaskCookies(cookieJSON string) (bool, string) {
	var cookies []capturedCookie
	if strings.TrimSpace(cookieJSON) == "" {
		return true, ""
	}
	if err := json.Unmarshal([]byte(cookieJSON), &cookies); err != nil {
		return true, ""
	}
	required := map[string]bool{"kwscode": false, "kwssectoken": false}
	now := float64(time.Now().Unix())
	for _, cookie := range cookies {
		if _, ok := required[cookie.Name]; !ok {
			continue
		}
		if cookie.Expires > 0 && cookie.Expires <= now+60 {
			return false, cookie.Name + " is expired; please re-login and capture the Kling account again"
		}
		required[cookie.Name] = true
	}
	for name, seen := range required {
		if !seen {
			return false, name + " is missing; please re-login and capture the Kling account again"
		}
	}
	return true, ""
}
