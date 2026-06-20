package internal

import "time"

func StartAccountKeepalive() {
	go func() {
		timer := time.NewTimer(2 * time.Minute)
		defer timer.Stop()
		<-timer.C
		for {
			runAccountKeepaliveOnce()
			time.Sleep(45 * time.Minute)
		}
	}()
}

func runAccountKeepaliveOnce() {
	if AppStore == nil {
		return
	}
	accounts, err := AppStore.ListAccounts()
	if err != nil {
		LogError("Kling account keepalive list failed: %v", err)
		return
	}
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		if account.CookieString == "" && !accountChromeProfileExists(account.ID) {
			continue
		}
		ok, message := testKlingAccount(&account)
		if err := AppStore.SetAccountTestResult(account.ID, ok, message); err != nil {
			LogError("Kling account keepalive update failed: %v", err)
		}
		if !ok {
			_ = AppStore.AddEvent(account.ID, "keepalive_failed", message, nil)
		}
	}
}
