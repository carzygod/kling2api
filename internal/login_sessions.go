package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
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
	err := chromedp.Run(s.ctx,
		network.Enable(),
		chromedp.Navigate(Cfg.LoginURL),
		chromedp.Sleep(3*time.Second),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	var image []byte
	err := chromedp.Run(s.ctx,
		chromedp.Sleep(500*time.Millisecond),
		chromedp.FullScreenshot(&image, 85),
	)
	if err != nil {
		s.setErrorLocked(err)
		return err
	}
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
	err := chromedp.Run(s.ctx,
		chromedp.MouseClickXY(x, y),
		chromedp.Sleep(800*time.Millisecond),
	)
	if err != nil {
		s.setError(err)
		return err
	}
	return s.RefreshScreenshot()
}

func (s *LoginSession) Input(text string) error {
	err := chromedp.Run(s.ctx,
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
	err := chromedp.Run(s.ctx,
		chromedp.Reload(),
		chromedp.Sleep(2*time.Second),
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
	err := chromedp.Run(s.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{
				"https://klingai.com/",
				"https://www.klingai.com/",
				"https://klingai.com/app",
				"https://app.klingai.com/",
			}).Do(ctx)
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
	account, err := AppStore.CreateAccount(AccountRecord{
		Name:             name,
		Status:           "captured",
		CookieJSON:       string(cookieJSON),
		CookieString:     cookieString,
		LocalStorageJSON: localStorageJSON,
		UserAgent:        userAgent,
	})
	if err != nil {
		s.setError(err)
		return nil, err
	}
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
	if a == nil || a.CookieString == "" {
		return false, "empty cookie string"
	}
	req, err := http.NewRequest(http.MethodGet, Cfg.LoginURL, nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Cookie", a.CookieString)
	if a.UserAgent != "" {
		req.Header.Set("User-Agent", a.UserAgent)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return false, fmt.Sprintf("klingai.com returned HTTP %d", resp.StatusCode)
	}
	return true, fmt.Sprintf("klingai.com returned HTTP %d; cookie transport is accepted", resp.StatusCode)
}
