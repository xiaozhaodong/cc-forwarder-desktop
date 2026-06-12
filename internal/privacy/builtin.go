package privacy

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	BuiltinEmail        = "builtin:email"
	BuiltinCNMobile     = "builtin:cn_mobile"
	BuiltinCNIDCard     = "builtin:cn_id_card"
	BuiltinBankCardLuhn = "builtin:bank_card_luhn"
)

type builtinMatcher func(text string) [][2]int

var (
	emailBuiltinRe    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	cnMobileBuiltinRe = regexp.MustCompile(`(?:\+86)?1[3-9]\d{9}`)
	cnIDCardBuiltinRe = regexp.MustCompile(`[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)
	bankCardBuiltinRe = regexp.MustCompile(`\d{13,19}`)
)

// BuiltinPIIRules 返回默认确定型 PII 内置规则定义。
func BuiltinPIIRules() []Rule {
	return []Rule{
		builtinRule("中国身份证号", "18 位格式、合法出生日期与校验位", BuiltinCNIDCard, "[身份证]", 20),
		builtinRule("银行卡号", "13-19 位数字并通过 Luhn 校验", BuiltinBankCardLuhn, "[银行卡]", 30),
		builtinRule("中国手机号", "中国大陆手机号，支持 +86 前缀", BuiltinCNMobile, "[手机号]", 40),
		builtinRule("邮箱地址", "基础邮箱格式，排除 git/ssh/scp 命令上下文", BuiltinEmail, "[邮箱]", 50),
	}
}

func builtinRule(name, description, pattern, placeholder string, priority int) Rule {
	return Rule{
		Enabled:     true,
		Name:        name,
		Description: description,
		Priority:    priority,
		MatchType:   MatchTypeBuiltin,
		Pattern:     pattern,
		Placeholder: placeholder,
		Action:      ActionRedact,
		Source:      SourceBuiltin,
	}
}

func compileBuiltinMatcher(pattern string) (builtinMatcher, error) {
	switch strings.TrimSpace(pattern) {
	case BuiltinEmail:
		return matchBuiltinEmail, nil
	case BuiltinCNMobile:
		return matchBuiltinCNMobile, nil
	case BuiltinCNIDCard:
		return matchBuiltinCNIDCard, nil
	case BuiltinBankCardLuhn:
		return matchBuiltinBankCardLuhn, nil
	default:
		return nil, fmt.Errorf("unknown builtin pattern: %s", pattern)
	}
}

func matchBuiltinEmail(text string) [][2]int {
	locs := emailBuiltinRe.FindAllStringIndex(text, -1)
	out := make([][2]int, 0, len(locs))
	for _, loc := range locs {
		if isCommandAddressContext(text, loc[0], loc[1]) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func isCommandAddressContext(text string, start, end int) bool {
	if end < len(text) && text[end] == ':' && strings.HasPrefix(strings.ToLower(text[start:end]), "git@") {
		return true
	}
	lineStart := strings.LastIndexByte(text[:start], '\n') + 1
	prefix := strings.ToLower(strings.TrimSpace(text[lineStart:start]))
	if prefix == "" {
		return false
	}
	fields := strings.Fields(prefix)
	if len(fields) == 0 {
		return false
	}
	last := fields[len(fields)-1]
	return last == "ssh" || last == "scp"
}

func matchBuiltinCNMobile(text string) [][2]int {
	locs := cnMobileBuiltinRe.FindAllStringIndex(text, -1)
	out := make([][2]int, 0, len(locs))
	for _, loc := range locs {
		if hasDigitBoundary(text, loc[0], loc[1]) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func matchBuiltinCNIDCard(text string) [][2]int {
	locs := cnIDCardBuiltinRe.FindAllStringIndex(text, -1)
	out := make([][2]int, 0, len(locs))
	for _, loc := range locs {
		if hasDigitBoundary(text, loc[0], loc[1]) {
			continue
		}
		value := text[loc[0]:loc[1]]
		if !validCNIDCard(value) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func matchBuiltinBankCardLuhn(text string) [][2]int {
	locs := bankCardBuiltinRe.FindAllStringIndex(text, -1)
	out := make([][2]int, 0, len(locs))
	for _, loc := range locs {
		if hasDigitBoundary(text, loc[0], loc[1]) {
			continue
		}
		value := text[loc[0]:loc[1]]
		if !validLuhn(value) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func hasDigitBoundary(text string, start, end int) bool {
	return (start > 0 && isASCIIDigit(text[start-1])) || (end < len(text) && isASCIIDigit(text[end]))
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func validCNIDCard(value string) bool {
	if len(value) != 18 {
		return false
	}
	birth := value[6:14]
	parsed, err := time.Parse("20060102", birth)
	if err != nil || parsed.Format("20060102") != birth {
		return false
	}
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checks := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		if !isASCIIDigit(value[i]) {
			return false
		}
		sum += int(value[i]-'0') * weights[i]
	}
	got := value[17]
	if got == 'x' {
		got = 'X'
	}
	return got == checks[sum%11]
}

func validLuhn(value string) bool {
	sum := 0
	double := false
	for i := len(value) - 1; i >= 0; i-- {
		if !isASCIIDigit(value[i]) {
			return false
		}
		digit := int(value[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum > 0 && sum%10 == 0
}
