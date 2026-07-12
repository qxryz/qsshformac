package ssh

import (
	"fmt"
	"regexp"
	"strings"
)

// shellQuote 将任意字符串安全地包裹为单引号 shell 参数，
// 通过 '\'' 转义内部单引号，杜绝命令注入。
//   abc        -> 'abc'
//   a'b        -> 'a'\''b'
//   x; rm -rf  -> 'x; rm -rf'（整体作为一个参数，不会被解释）
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var (
	// 单元/服务名、firewalld zone 等标识符：字母数字、下划线、连字符、点
	reIdentifier = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// 端口或端口范围：80 或 8000:9000
	rePort = regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`)
	// IP / CIDR：粗校验，交给远端 iptables/ufw 做最终判断
	reCIDR = regexp.MustCompile(`^[0-9a-fA-F:.]+(/[0-9]{1,3})?$`)
	// kill 信号：数字或大写信号名（可带 SIG 前缀）
	reSignal = regexp.MustCompile(`^(SIG)?[A-Z]+$|^[0-9]{1,2}$`)
)

// validIdentifier 校验标识符（服务名/zone 等），非法返回错误。
func validIdentifier(field, v string) error {
	if !reIdentifier.MatchString(v) {
		return fmt.Errorf("非法的%s: %q", field, v)
	}
	return nil
}

// validPort 校验端口或端口范围。
func validPort(v string) error {
	if !rePort.MatchString(v) {
		return fmt.Errorf("非法端口: %q", v)
	}
	return nil
}

// validCIDR 校验 IP/CIDR。
func validCIDR(v string) error {
	if !reCIDR.MatchString(v) {
		return fmt.Errorf("非法的来源地址: %q", v)
	}
	return nil
}

// validSignal 校验 kill 信号名。
func validSignal(v string) error {
	if !reSignal.MatchString(v) {
		return fmt.Errorf("非法信号: %q", v)
	}
	return nil
}

// oneOf 校验 v 是否在允许集合内（大小写不敏感返回规范化值）。
func oneOf(field, v string, allowed ...string) (string, error) {
	lv := strings.ToLower(strings.TrimSpace(v))
	for _, a := range allowed {
		if lv == a {
			return a, nil
		}
	}
	return "", fmt.Errorf("非法的%s: %q", field, v)
}
