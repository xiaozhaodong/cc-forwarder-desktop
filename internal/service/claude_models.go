package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CategoryClaudeModels    = "claude_models"
	ClaudeModelsCatalogKey  = "catalog"
	MaxClaudeModelEntries   = 1000
	maxClaudeModelTextRunes = 256
)

// ClaudeModelEntry 表示 Claude Code Gateway Discovery 可展示的一个模型。
// 这里只维护展示目录，不声明端点能力，也不参与调度。
type ClaudeModelEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

// ClaudeModelCatalog 是持久化在 settings 中的 Gateway Discovery 展示目录。
type ClaudeModelCatalog struct {
	Enabled bool               `json:"enabled"`
	Models  []ClaudeModelEntry `json:"models"`
}

type ClaudeModelsListResponse struct {
	Data []ClaudeModelsListObject `json:"data"`
}

type ClaudeModelsListObject struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func DefaultClaudeModelCatalog() ClaudeModelCatalog {
	return ClaudeModelCatalog{Enabled: true, Models: []ClaudeModelEntry{}}
}

func DecodeClaudeModelCatalog(raw string) (ClaudeModelCatalog, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultClaudeModelCatalog(), nil
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var catalog ClaudeModelCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return ClaudeModelCatalog{}, fmt.Errorf("解析 Claude 模型目录失败: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ClaudeModelCatalog{}, fmt.Errorf("解析 Claude 模型目录失败: 存在多余 JSON 内容")
		}
		return ClaudeModelCatalog{}, fmt.Errorf("解析 Claude 模型目录失败: %w", err)
	}
	return NormalizeClaudeModelCatalog(catalog)
}

func NormalizeClaudeModelCatalog(catalog ClaudeModelCatalog) (ClaudeModelCatalog, error) {
	if len(catalog.Models) > MaxClaudeModelEntries {
		return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型目录最多允许 %d 个条目", MaxClaudeModelEntries)
	}

	models := make([]ClaudeModelEntry, 0, len(catalog.Models))
	seen := make(map[string]struct{}, len(catalog.Models))
	for index, model := range catalog.Models {
		model.ID = strings.TrimSpace(model.ID)
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		if model.ID == "" {
			return ClaudeModelCatalog{}, fmt.Errorf("第 %d 个 Claude 模型缺少 id", index+1)
		}
		if utf8.RuneCountInString(model.ID) > maxClaudeModelTextRunes {
			return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型 id %q 超过 %d 个字符", model.ID, maxClaudeModelTextRunes)
		}
		if strings.IndexFunc(model.ID, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型 id %q 不能包含空白或控制字符", model.ID)
		}
		if _, exists := seen[model.ID]; exists {
			return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型 id %q 重复", model.ID)
		}
		seen[model.ID] = struct{}{}

		if model.DisplayName == "" {
			model.DisplayName = model.ID
		}
		if utf8.RuneCountInString(model.DisplayName) > maxClaudeModelTextRunes {
			return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型 %q 的显示名称超过 %d 个字符", model.ID, maxClaudeModelTextRunes)
		}
		if strings.IndexFunc(model.DisplayName, unicode.IsControl) >= 0 {
			return ClaudeModelCatalog{}, fmt.Errorf("Claude 模型 %q 的显示名称不能包含控制字符", model.ID)
		}
		models = append(models, model)
	}

	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Enabled != models[j].Enabled {
			return models[i].Enabled
		}
		return models[i].ID < models[j].ID
	})
	catalog.Models = models
	return catalog, nil
}

func (c ClaudeModelCatalog) BuildListResponse() ClaudeModelsListResponse {
	data := make([]ClaudeModelsListObject, 0, len(c.Models))
	if c.Enabled {
		for _, model := range c.Models {
			if !model.Enabled {
				continue
			}
			data = append(data, ClaudeModelsListObject{ID: model.ID, DisplayName: model.DisplayName})
		}
	}
	return ClaudeModelsListResponse{Data: data}
}

func (s *SettingsService) GetClaudeModelCatalog(ctx context.Context) (ClaudeModelCatalog, error) {
	if s == nil || s.store == nil {
		return DefaultClaudeModelCatalog(), nil
	}
	raw, err := s.GetValue(ctx, CategoryClaudeModels, ClaudeModelsCatalogKey)
	if err != nil {
		return ClaudeModelCatalog{}, err
	}
	return DecodeClaudeModelCatalog(raw)
}
