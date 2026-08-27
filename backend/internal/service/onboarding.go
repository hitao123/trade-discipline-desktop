package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type CompleteOnboardingInput struct {
	InvestableCapitalFen int64                 `json:"investableCapitalFen"`
	MaxLossFen           int64                 `json:"maxLossFen"`
	HoldingHorizon       domain.HoldingHorizon `json:"holdingHorizon"`
	EnabledMarkets       []domain.MarketScope  `json:"enabledMarkets"`
}

type UpdateUserProfileInput struct {
	CompleteOnboardingInput
	Reason string `json:"reason"`
}

func (s *Service) Onboarding(ctx context.Context) (domain.UserProfile, error) {
	return s.store.UserProfile(ctx)
}

func (s *Service) CompleteOnboarding(ctx context.Context, input CompleteOnboardingInput) (domain.UserProfile, error) {
	if err := validateOnboardingInput(input); err != nil {
		return domain.UserProfile{}, err
	}
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return domain.UserProfile{}, err
	}
	if profile.Mode != domain.UserModeGeneric || profile.OnboardingStatus != domain.OnboardingPending {
		return domain.UserProfile{}, CodedError{Code: "ONBOARDING_ALREADY_COMPLETED", Message: store.ErrOnboardingAlreadyCompleted.Error()}
	}
	profile.InvestableCapitalFen = input.InvestableCapitalFen
	profile.MaxLossFen = input.MaxLossFen
	profile.HoldingHorizon = input.HoldingHorizon
	profile.EnabledMarkets = normalizedMarketScopes(input.EnabledMarkets)
	completed, err := s.store.CompleteGenericOnboarding(ctx, profile, rules.GenericSnapshot(input.InvestableCapitalFen, input.MaxLossFen), s.now())
	if errors.Is(err, store.ErrOnboardingAlreadyCompleted) {
		return domain.UserProfile{}, CodedError{Code: "ONBOARDING_ALREADY_COMPLETED", Message: err.Error()}
	}
	return completed, err
}

func (s *Service) UpdateUserProfile(ctx context.Context, input UpdateUserProfileInput) (domain.UserProfile, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return domain.UserProfile{}, CodedError{Code: "INVALID_PROFILE", Message: "请填写修改原因"}
	}
	if err := validateOnboardingInput(input.CompleteOnboardingInput); err != nil {
		return domain.UserProfile{}, CodedError{Code: "INVALID_PROFILE", Message: err.Error()}
	}
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return domain.UserProfile{}, err
	}
	if profile.Mode != domain.UserModeGeneric || profile.OnboardingStatus != domain.OnboardingCompleted {
		return domain.UserProfile{}, CodedError{Code: "PROFILE_MODE_CONFLICT", Message: store.ErrGenericProfileRequired.Error()}
	}
	current, err := s.store.CurrentRule(ctx)
	if err != nil {
		return domain.UserProfile{}, err
	}
	next := rules.GenericSnapshot(input.InvestableCapitalFen, input.MaxLossFen)
	next.HKBoardLot = current.HKBoardLot
	next.HKDCNYRateBP = current.HKDCNYRateBP
	next.NoAddToLosingInstrument = current.NoAddToLosingInstrument
	next.ExitCodes = append([]string(nil), current.ExitCodes...)
	next.Cooldown = current.Cooldown
	profile.InvestableCapitalFen = input.InvestableCapitalFen
	profile.MaxLossFen = input.MaxLossFen
	profile.HoldingHorizon = input.HoldingHorizon
	profile.EnabledMarkets = normalizedMarketScopes(input.EnabledMarkets)
	if err := s.createAutomaticBackup(ctx, "profile"); err != nil {
		return domain.UserProfile{}, err
	}
	updated, err := s.store.UpdateGenericProfile(ctx, profile, next, strings.TrimSpace(input.Reason), s.now())
	if errors.Is(err, store.ErrGenericProfileRequired) {
		return domain.UserProfile{}, CodedError{Code: "PROFILE_MODE_CONFLICT", Message: err.Error()}
	}
	return updated, err
}

func validateOnboardingInput(input CompleteOnboardingInput) error {
	if input.InvestableCapitalFen <= 0 {
		return CodedError{Code: "INVALID_ONBOARDING", Message: "可投资总资金必须大于 0"}
	}
	if input.MaxLossFen <= 0 || input.MaxLossFen >= input.InvestableCapitalFen {
		return CodedError{Code: "INVALID_ONBOARDING", Message: "最大可承受损失必须大于 0 且小于总资金"}
	}
	validHorizon := map[domain.HoldingHorizon]bool{
		domain.HorizonUnder6M: true,
		domain.Horizon6To12M:  true,
		domain.Horizon1To3Y:   true,
		domain.HorizonOver3Y:  true,
	}
	if !validHorizon[input.HoldingHorizon] {
		return CodedError{Code: "INVALID_ONBOARDING", Message: "请选择预期持有期限"}
	}
	if len(normalizedMarketScopes(input.EnabledMarkets)) == 0 {
		return CodedError{Code: "INVALID_ONBOARDING", Message: "请至少选择一个使用市场"}
	}
	for _, scope := range input.EnabledMarkets {
		if scope != domain.MarketAShareStock && scope != domain.MarketAShareETF && scope != domain.MarketHK {
			return CodedError{Code: "INVALID_ONBOARDING", Message: fmt.Sprintf("不支持的市场范围：%s", scope)}
		}
	}
	return nil
}

func normalizedMarketScopes(scopes []domain.MarketScope) []domain.MarketScope {
	present := make(map[domain.MarketScope]bool, len(scopes))
	for _, scope := range scopes {
		present[scope] = true
	}
	result := make([]domain.MarketScope, 0, 3)
	for _, scope := range []domain.MarketScope{domain.MarketAShareStock, domain.MarketAShareETF, domain.MarketHK} {
		if present[scope] {
			result = append(result, scope)
		}
	}
	return result
}
