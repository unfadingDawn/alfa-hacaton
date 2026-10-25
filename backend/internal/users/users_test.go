package users

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateProfileOtherRole(t *testing.T) {
	req := profileReq{FullName: "Иван Петров", RoleCode: "other"}
	if errs := ValidateProfile(&req); errs["position_title"] == "" {
		t.Fatal("для «Иного сотрудника» роль вводит пользователь")
	}
	req = profileReq{FullName: "Иван Петров", RoleCode: "other", PositionTitle: "Директор по ВЭД"}
	if errs := ValidateProfile(&req); len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
	req = profileReq{FullName: "Иван Петров", RoleCode: "cfo", PositionTitle: "что угодно"}
	_ = ValidateProfile(&req)
	if req.PositionTitle != "Финансовый директор" {
		t.Fatalf("для стандартной роли используется её название: %q", req.PositionTitle)
	}
}

func TestValidateAvatar(t *testing.T) {
	ok := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 200)))
	if msg := ValidateAvatar(ok); msg != "" {
		t.Fatal(msg)
	}
	for _, bad := range []string{"data:image/svg+xml;base64,AAAA", "data:image/png;base64,@@@", "http://x/y.png",
		"data:image/png;base64," + strings.Repeat("A", 500*1024)} {
		if ValidateAvatar(bad) == "" {
			t.Errorf("должно отклоняться: %.40s", bad)
		}
	}
}
