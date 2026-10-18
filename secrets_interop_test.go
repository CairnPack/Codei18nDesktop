//go:build windows

package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/CairnPack/codei18n/core/secrets"
)

// 互通测试（真实 Windows 凭据管理器）：用 mygo.Secrets.Service("codei18n") 写入的令牌，core 能读出；
// core 写入的令牌，mygo 也能读出。两者格式一致：generic credential，target "codei18n/<account>"，UserName "<account>"。
// 设置 CODEI18N_SKIP_CREDMAN=1 可跳过。
func TestMygoSecretsInteropWithCore(t *testing.T) {
	if os.Getenv("CODEI18N_SKIP_CREDMAN") != "" {
		t.Skip("CODEI18N_SKIP_CREDMAN 已设置")
	}
	mg := mygo.Secrets.Service(secrets.Service)
	store := secrets.System()
	account := fmt.Sprintf("test-interop-%d/profile/demo", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = mg.Delete(account)
		_ = store.Delete(account)
	})

	if err := mg.Set(account, []byte("mygo-written-token-1234")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(account)
	if err != nil || got.Reveal() != "mygo-written-token-1234" {
		t.Fatalf("core 读不到 mygo 写入的令牌: %v", err)
	}

	if err := store.Set(account, secrets.NewSecret([]byte("core-written-token-5678"))); err != nil {
		t.Fatal(err)
	}
	raw, err := mg.Get(account)
	if err != nil || string(raw) != "core-written-token-5678" {
		t.Fatalf("mygo 读不到 core 写入的令牌: %v", err)
	}
}
