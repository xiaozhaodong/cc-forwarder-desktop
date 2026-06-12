package service

import (
	"context"

	"cc-forwarder/internal/privacy"
	"cc-forwarder/internal/store"
)

func (s *PrivacyService) ensureBuiltinRulesLocked(ctx context.Context, existing []*store.PrivacyRuleRecord) error {
	existingBuiltin := make(map[string]struct{}, len(existing))
	for _, record := range existing {
		if record == nil {
			continue
		}
		if record.Source == privacy.SourceBuiltin && record.MatchType == privacy.MatchTypeBuiltin {
			existingBuiltin[record.Pattern] = struct{}{}
		}
	}

	var missing []*store.PrivacyRuleRecord
	for _, rule := range privacy.BuiltinPIIRules() {
		if _, ok := existingBuiltin[rule.Pattern]; ok {
			continue
		}
		scopeJSON, err := privacy.EncodeScope(rule.Scope)
		if err != nil {
			return err
		}
		missing = append(missing, &store.PrivacyRuleRecord{
			Enabled:     rule.Enabled,
			Name:        rule.Name,
			Description: rule.Description,
			Priority:    rule.Priority,
			MatchType:   rule.MatchType,
			Pattern:     rule.Pattern,
			Placeholder: rule.Placeholder,
			Action:      rule.Action,
			ScopeJSON:   scopeJSON,
			Source:      privacy.SourceBuiltin,
		})
	}
	if len(missing) == 0 {
		return nil
	}
	if _, err := s.store.CreateRules(ctx, missing); err != nil {
		return err
	}
	return nil
}
