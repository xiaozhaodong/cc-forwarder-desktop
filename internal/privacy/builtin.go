package privacy

import (
	"fmt"
	"regexp"
	"strconv"
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
	emailBuiltinRe               = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	emailImageScaleAssetSuffixRe = regexp.MustCompile(`(?i)@[1-9][0-9]*(?:\.[0-9]+)?x\.(?:png|jpe?g|webp|gif|svg|avif|heic|heif|bmp|tiff?)$`)
	cnMobileBuiltinRe            = regexp.MustCompile(`(?:\+86)?(?:1740[0-5]\d{6}|1(?:[38]\d|4[57]|[59][0-35-9]|6[25-7]|7[0-35-8])\d{8})`)
	cnIDCardBuiltinRe            = regexp.MustCompile(`[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)
	bankCardBuiltinRe            = regexp.MustCompile(`\d{13,19}`)
)

// BuiltinPIIRules 返回默认确定型 PII 内置规则定义。
func BuiltinPIIRules() []Rule {
	return []Rule{
		builtinRule("中国身份证号", "18 位格式、省级地址码、合法出生日期与校验位", BuiltinCNIDCard, "[身份证]", 20),
		builtinRule("银行卡号", "主流卡组织前缀/长度匹配，并通过 Luhn 校验", BuiltinBankCardLuhn, "[银行卡]", 30),
		builtinRule("中国手机号", "中国大陆手机号段格式，支持 +86 前缀", BuiltinCNMobile, "[手机号]", 40),
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
		if isImageScaleAssetName(text[loc[0]:loc[1]]) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func isImageScaleAssetName(value string) bool {
	return emailImageScaleAssetSuffixRe.MatchString(value)
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
		if !matchesKnownPaymentCardProfile(value) {
			continue
		}
		if !validLuhn(value) {
			continue
		}
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

func matchesKnownPaymentCardProfile(value string) bool {
	length := len(value)
	switch len(value) {
	case 13, 14, 15, 16, 17, 18, 19:
	default:
		return false
	}

	return matchesVisa(value, length) ||
		matchesMastercard(value, length) ||
		matchesAmericanExpress(value, length) ||
		matchesDinersClub(value, length) ||
		matchesDiscover(value, length) ||
		matchesJCB(value, length) ||
		matchesUnionPay(value, length)
}

func matchesVisa(value string, length int) bool {
	return strings.HasPrefix(value, "4") && hasLength(length, 13, 16, 18, 19)
}

func matchesMastercard(value string, length int) bool {
	if length != 16 {
		return false
	}
	prefix2 := prefixInt(value, 2)
	if prefix2 >= 51 && prefix2 <= 55 {
		return true
	}
	prefix4 := prefixInt(value, 4)
	return prefix4 >= 2221 && prefix4 <= 2720
}

func matchesAmericanExpress(value string, length int) bool {
	if length != 15 {
		return false
	}
	prefix2 := prefixInt(value, 2)
	return prefix2 == 34 || prefix2 == 37
}

func matchesDinersClub(value string, length int) bool {
	if length != 14 {
		return false
	}
	prefix2 := prefixInt(value, 2)
	prefix3 := prefixInt(value, 3)
	return (prefix3 >= 300 && prefix3 <= 305) || prefix2 == 36 || prefix2 == 38 || prefix2 == 39
}

func matchesDiscover(value string, length int) bool {
	if !hasLength(length, 16, 19) {
		return false
	}
	prefix2 := prefixInt(value, 2)
	prefix3 := prefixInt(value, 3)
	prefix4 := prefixInt(value, 4)
	return prefix4 == 6011 || (prefix3 >= 644 && prefix3 <= 649) || prefix2 == 65
}

func matchesJCB(value string, length int) bool {
	prefix4 := prefixInt(value, 4)
	if length == 15 && (prefix4 == 2131 || prefix4 == 1800) {
		return true
	}
	return length >= 16 && length <= 19 && prefix4 >= 3528 && prefix4 <= 3589
}

func matchesUnionPay(value string, length int) bool {
	if length < 14 || length > 19 {
		return false
	}
	prefix3 := prefixInt(value, 3)
	if prefix3 == 620 || (prefix3 >= 623 && prefix3 <= 626) || prefix3 == 810 {
		return true
	}
	prefix4 := prefixInt(value, 4)
	if prefix4 == 6270 || prefix4 == 6272 || prefix4 == 6276 ||
		(prefix4 >= 6282 && prefix4 <= 6289) || prefix4 == 6291 || prefix4 == 6292 ||
		(prefix4 >= 8110 && prefix4 <= 8171) {
		return true
	}
	prefix5 := prefixInt(value, 5)
	if (prefix5 >= 62100 && prefix5 <= 62182) ||
		(prefix5 >= 62184 && prefix5 <= 62197) ||
		(prefix5 >= 62200 && prefix5 <= 62205) ||
		(prefix5 >= 62207 && prefix5 <= 62209) {
		return true
	}
	prefix6 := prefixInt(value, 6)
	return (prefix6 >= 622010 && prefix6 <= 622999) ||
		(prefix6 >= 627700 && prefix6 <= 627779) ||
		(prefix6 >= 627781 && prefix6 <= 627799)
}

func hasLength(length int, allowed ...int) bool {
	for _, item := range allowed {
		if length == item {
			return true
		}
	}
	return false
}

func prefixInt(value string, length int) int {
	if len(value) < length {
		return -1
	}
	prefix, err := strconv.Atoi(value[:length])
	if err != nil {
		return -1
	}
	return prefix
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
	if !validCNIDProvinceCode(value[:2]) {
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

func validCNIDProvinceCode(value string) bool {
	switch value {
	case "11", "12", "13", "14", "15",
		"21", "22", "23",
		"31", "32", "33", "34", "35", "36", "37",
		"41", "42", "43", "44", "45", "46",
		"50", "51", "52", "53", "54",
		"61", "62", "63", "64", "65",
		"71", "81", "82":
		return true
	default:
		return false
	}
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
