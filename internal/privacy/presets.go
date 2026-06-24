package privacy

// Preset 内置预设规则集
type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Rules       []Rule `json:"rules"`
}

// 预设 ID
const (
	PresetBasicPrivacy = "basic-privacy"
	PresetAIAPIKeys    = "ai-api-keys"
)

// Presets 返回内置预设。gitleaks 完整规则集暂不内置（见实施计划 5.7）。
func Presets() []Preset {
	return []Preset{
		{
			ID:          PresetBasicPrivacy,
			Name:        "基础隐私",
			Description: "确定型 PII：中国手机号、身份证、银行卡",
			Rules:       BuiltinPIIRules(),
		},
		{
			ID:          PresetAIAPIKeys,
			Name:        "常见 AI/API Key",
			Description: "密码字段、通用密钥字段、OpenAI、Anthropic、GitHub PAT、AWS、Google、高德地图、Slack、JWT、私钥头",
			Rules: []Rule{
				presetRule("密码字段", `(?i)(?:\b(?:password|passwd|pwd|passphrase)\b|密码|口令)\s*[:=：]\s*['"]?(?P<redact>[^'"\s,;]{6,})['"]?`, "[密码]", 80),
				presetRule("通用密钥字段", `(?i)(?:\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|id[_-]?token|client[_-]?secret|secret[_-]?key|secret)\b|密钥|令牌|访问令牌|刷新令牌|客户端密钥)\s*[:=：]\s*['"]?(?P<redact>[A-Za-z0-9._~+/=-]{12,})['"]?`, "[密钥]", 90),
				presetRule("高德地图 Key", `(?i)\b(?:amap|gaode)[._-](?:web[._-]?service[._-]?)?(?:api[._-]?)?key\b\s*[:=：]\s*['"]?(?P<redact>[A-Za-z0-9]{16,64})['"]?`, "[高德地图密钥]", 92),
				presetRule("Bearer Token", `(?i)\bBearer\s+(?P<redact>[A-Za-z0-9._~+/=-]{20,})`, "[Bearer令牌]", 95),
				presetRule("Basic Auth Token", `(?i)\bBasic\s+(?P<redact>[A-Za-z0-9+/=]{16,})`, "[Basic认证]", 96),
				presetRule("含凭据连接串", `(?i)\b(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp|ftp)://[^:'"\s/@]+:[^@'"\s]+@[^'"\s]+`, "[含凭据连接串]", 98),
				// Anthropic 必须排在 OpenAI 之前：sk-ant-... 同时满足 OpenAI 的宽松模式
				presetRule("Anthropic API Key", `sk-ant-[A-Za-z0-9_-]{20,}`, "[Anthropic密钥]", 100),
				presetRule("OpenAI API Key", `sk-(?:proj-)?[A-Za-z0-9_-]{20,}`, "[OpenAI密钥]", 110),
				presetRule("GitHub Token", `(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})`, "[GitHub令牌]", 120),
				presetRule("AWS Access Key", `\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`, "[AWS密钥]", 130),
				presetRule("Google API Key", `\bAIza[0-9A-Za-z_-]{35}\b`, "[Google密钥]", 140),
				presetRule("Slack Token", `xox[baprs]-[A-Za-z0-9-]{10,}`, "[Slack令牌]", 150),
				presetRule("Hugging Face Token", `\bhf_[A-Za-z0-9]{20,}\b`, "[HuggingFace令牌]", 160),
				presetRule("Telegram Bot Token", `\b\d{8,10}:[A-Za-z0-9_-]{35}\b`, "[Telegram令牌]", 170),
				presetRule("JWT", `\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{5,}`, "[JWT]", 180),
				presetRule("私钥块头部", `-----BEGIN [A-Z ]*PRIVATE KEY-----`, "[私钥]", 190),
				presetRule("IPv4 地址", `\b(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(?:\.(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}\b`, "[IP地址]", 240),
			},
		},
	}
}

// FindPreset 按 ID 查找预设
func FindPreset(presetID string) (Preset, bool) {
	for _, preset := range Presets() {
		if preset.ID == presetID {
			return preset, true
		}
	}
	return Preset{}, false
}

func presetRule(name, pattern, placeholder string, priority int) Rule {
	return Rule{
		Enabled:     true,
		Name:        name,
		Priority:    priority,
		MatchType:   MatchTypeRegex,
		Pattern:     pattern,
		Placeholder: placeholder,
		Action:      ActionRedact,
		Source:      SourcePreset,
	}
}
