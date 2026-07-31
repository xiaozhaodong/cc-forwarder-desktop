// Package modelrewrite 提供账号与端点共用的模型改写规则解析和匹配能力。
package modelrewrite

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MatchExact  = "exact"
	MatchPrefix = "prefix"
)

// Rule 描述一条按请求路径和模型名执行的改写规则。
type Rule struct {
	Paths []string `json:"paths"`
	Match string   `json:"match"`
	From  string   `json:"from"`
	To    string   `json:"to"`
}

// Parse 兼容规则数组和历史单对象格式。
func Parse(raw string) ([]Rule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var rules []Rule
	if err := json.Unmarshal([]byte(raw), &rules); err == nil {
		return rules, nil
	}

	var single Rule
	if err := json.Unmarshal([]byte(raw), &single); err != nil {
		return nil, fmt.Errorf("解析模型改写规则失败: %w", err)
	}
	return []Rule{single}, nil
}

// Rewrite 返回命中规则后的模型名。运行时保留 prefix 兼容，但新配置应使用 exact。
func Rewrite(raw, path, model string) (string, bool, error) {
	model = strings.TrimSpace(model)
	if strings.TrimSpace(raw) == "" || model == "" {
		return "", false, nil
	}

	rules, err := Parse(raw)
	if err != nil {
		return "", false, err
	}
	for _, rule := range rules {
		target := strings.TrimSpace(rule.To)
		if target == "" || !matchesPath(rule, path) || !matchesModel(rule, model) {
			continue
		}
		if target == model {
			return "", false, nil
		}
		return target, true, nil
	}
	return "", false, nil
}

// ValidateExact 校验前端可维护的精确规则，并要求每条规则覆盖全部指定路径。
func ValidateExact(raw string, allowedPaths ...string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	rules, err := Parse(raw)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return fmt.Errorf("模型改写规则不能为空")
	}

	allowed := make(map[string]struct{}, len(allowedPaths))
	for _, path := range allowedPaths {
		if path = strings.TrimSpace(path); path != "" {
			allowed[path] = struct{}{}
		}
	}

	for index, rule := range rules {
		from := strings.TrimSpace(rule.From)
		to := strings.TrimSpace(rule.To)
		match := strings.ToLower(strings.TrimSpace(rule.Match))
		if match == "" {
			match = MatchExact
		}
		if match != MatchExact {
			return fmt.Errorf("第 %d 条模型改写规则只允许 exact 精确匹配", index+1)
		}
		if from == "" || to == "" {
			return fmt.Errorf("第 %d 条模型改写规则必须填写来源模型和目标模型", index+1)
		}
		if strings.EqualFold(from, to) {
			return fmt.Errorf("第 %d 条模型改写规则的来源模型和目标模型不能相同", index+1)
		}
		if len(rule.Paths) == 0 {
			return fmt.Errorf("第 %d 条模型改写规则必须指定请求路径", index+1)
		}
		seenPaths := make(map[string]struct{}, len(rule.Paths))
		for _, path := range rule.Paths {
			path = strings.TrimSpace(path)
			if _, ok := allowed[path]; !ok {
				return fmt.Errorf("第 %d 条模型改写规则包含不支持的请求路径 %q", index+1, path)
			}
			if _, duplicated := seenPaths[path]; duplicated {
				return fmt.Errorf("第 %d 条模型改写规则包含重复的请求路径 %q", index+1, path)
			}
			seenPaths[path] = struct{}{}
		}
		for _, path := range allowedPaths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if _, ok := seenPaths[path]; !ok {
				return fmt.Errorf("第 %d 条模型改写规则缺少请求路径 %q", index+1, path)
			}
		}
	}
	return nil
}

func matchesPath(rule Rule, path string) bool {
	if len(rule.Paths) == 0 {
		return true
	}
	for _, allowedPath := range rule.Paths {
		if strings.TrimSpace(allowedPath) == path {
			return true
		}
	}
	return false
}

func matchesModel(rule Rule, model string) bool {
	from := strings.TrimSpace(rule.From)
	if from == "" {
		return false
	}
	match := strings.ToLower(strings.TrimSpace(rule.Match))
	if match == "" {
		match = MatchExact
	}

	normalizedModel := strings.ToLower(model)
	normalizedFrom := strings.ToLower(from)
	switch match {
	case MatchPrefix:
		return strings.HasPrefix(normalizedModel, normalizedFrom)
	case MatchExact:
		return normalizedModel == normalizedFrom
	default:
		return false
	}
}
