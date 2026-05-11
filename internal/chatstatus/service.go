package chatstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type Store interface {
	UpsertMessengerChatUser(context.Context, domain.MessengerChatUser) error
	SaveMessengerChatUserCheck(context.Context, domain.MessengerChatUserCheck) error
}

type TelegramChecker interface {
	GetChatMember(context.Context, string, int64) (telegram.ChatMemberInfo, error)
}

type MAXChecker interface {
	GetChatMembers(context.Context, int64, []int64) (max.ChatMembersPage, error)
}

type Service struct {
	Store    Store
	Telegram TelegramChecker
	MAX      MAXChecker
	Now      func() time.Time
}

type RefreshRequest struct {
	MessengerKind   domain.MessengerKind
	ChatRef         string
	MessengerUserID string
	UserID          int64
	ConnectorID     int64
	SubscriptionID  int64
	Source          string
}

func (s Service) Refresh(ctx context.Context, req RefreshRequest) (domain.MessengerChatUser, error) {
	now := s.now()
	if strings.TrimSpace(req.Source) == "" {
		req.Source = "manual"
	}
	switch req.MessengerKind {
	case domain.MessengerKindTelegram:
		return s.refreshTelegram(ctx, req, now)
	case domain.MessengerKindMAX:
		return s.refreshMAX(ctx, req, now)
	default:
		return domain.MessengerChatUser{}, fmt.Errorf("unsupported messenger kind: %s", req.MessengerKind)
	}
}

func (s Service) refreshTelegram(ctx context.Context, req RefreshRequest, now time.Time) (domain.MessengerChatUser, error) {
	if s.Telegram == nil {
		return domain.MessengerChatUser{}, fmt.Errorf("telegram checker is not configured")
	}
	telegramID, err := strconv.ParseInt(strings.TrimSpace(req.MessengerUserID), 10, 64)
	if err != nil || telegramID <= 0 {
		return domain.MessengerChatUser{}, fmt.Errorf("invalid telegram user id: %q", req.MessengerUserID)
	}
	info, err := s.Telegram.GetChatMember(ctx, req.ChatRef, telegramID)
	if err != nil {
		state := errorState(req, now, err)
		if persistErr := s.persist(ctx, state, checkFromState(req, state, false, nil, err.Error(), now)); persistErr != nil {
			return state, persistErr
		}
		return state, err
	}
	state := stateFromTelegram(req, info, now)
	return state, s.persist(ctx, state, checkFromState(req, state, true, info.Raw, "", now))
}

func (s Service) refreshMAX(ctx context.Context, req RefreshRequest, now time.Time) (domain.MessengerChatUser, error) {
	if s.MAX == nil {
		return domain.MessengerChatUser{}, fmt.Errorf("max checker is not configured")
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(req.ChatRef), 10, 64)
	if err != nil || chatID == 0 {
		return domain.MessengerChatUser{}, fmt.Errorf("invalid max chat id: %q", req.ChatRef)
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(req.MessengerUserID), 10, 64)
	if err != nil || userID <= 0 {
		return domain.MessengerChatUser{}, fmt.Errorf("invalid max user id: %q", req.MessengerUserID)
	}
	page, err := s.MAX.GetChatMembers(ctx, chatID, []int64{userID})
	if err != nil {
		state := errorState(req, now, err)
		if persistErr := s.persist(ctx, state, checkFromState(req, state, false, nil, err.Error(), now)); persistErr != nil {
			return state, persistErr
		}
		return state, err
	}
	state := stateFromMAX(req, page, userID, now)
	return state, s.persist(ctx, state, checkFromState(req, state, true, page.Raw, "", now))
}

func (s Service) persist(ctx context.Context, state domain.MessengerChatUser, check domain.MessengerChatUserCheck) error {
	if s.Store == nil {
		return fmt.Errorf("chat status store is not configured")
	}
	if err := s.Store.UpsertMessengerChatUser(ctx, state); err != nil {
		return err
	}
	return s.Store.SaveMessengerChatUserCheck(ctx, check)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func stateFromTelegram(req RefreshRequest, info telegram.ChatMemberInfo, now time.Time) domain.MessengerChatUser {
	status := normalizeTelegramStatus(info.Status)
	state := baseState(req, status, now)
	state.Permissions = append([]string(nil), info.Permissions...)
	switch status {
	case domain.MessengerChatUserStatusOwner:
		state.IsMember = true
		state.IsOwner = true
		t := now
		state.LastJoinedAt = &t
	case domain.MessengerChatUserStatusAdministrator:
		state.IsMember = true
		state.IsAdmin = true
		t := now
		state.LastJoinedAt = &t
	case domain.MessengerChatUserStatusMember:
		state.IsMember = true
		t := now
		state.LastJoinedAt = &t
	case domain.MessengerChatUserStatusRestricted:
		state.IsRestricted = true
		state.IsMember = true
		t := now
		state.LastJoinedAt = &t
	case domain.MessengerChatUserStatusLeft:
		t := now
		state.LastLeftAt = &t
	case domain.MessengerChatUserStatusBanned:
		state.IsBanned = true
		t := now
		state.LastKickedAt = &t
	}
	return state
}

func stateFromMAX(req RefreshRequest, page max.ChatMembersPage, userID int64, now time.Time) domain.MessengerChatUser {
	var member *max.ChatMember
	for i := range page.Members {
		if page.Members[i].UserID == userID {
			member = &page.Members[i]
			break
		}
	}
	if member == nil {
		state := baseState(req, domain.MessengerChatUserStatusNotMember, now)
		t := now
		state.LastLeftAt = &t
		return state
	}
	status := domain.MessengerChatUserStatusMember
	if member.IsOwner {
		status = domain.MessengerChatUserStatusOwner
	} else if member.IsAdmin {
		status = domain.MessengerChatUserStatusAdministrator
	}
	state := baseState(req, status, now)
	state.IsMember = true
	state.IsOwner = member.IsOwner
	state.IsAdmin = member.IsAdmin
	state.Permissions = append([]string(nil), member.Permissions...)
	t := now
	state.LastJoinedAt = &t
	return state
}

func errorState(req RefreshRequest, now time.Time, err error) domain.MessengerChatUser {
	state := baseState(req, domain.MessengerChatUserStatusError, now)
	state.LastError = strings.TrimSpace(err.Error())
	return state
}

func baseState(req RefreshRequest, status domain.MessengerChatUserStatus, now time.Time) domain.MessengerChatUser {
	return domain.MessengerChatUser{
		MessengerKind:   req.MessengerKind,
		ChatRef:         strings.TrimSpace(req.ChatRef),
		MessengerUserID: strings.TrimSpace(req.MessengerUserID),
		UserID:          req.UserID,
		Status:          status,
		CheckedAt:       now,
		UpdatedAt:       now,
	}
}

func checkFromState(req RefreshRequest, state domain.MessengerChatUser, ok bool, rawJSON []byte, rawError string, now time.Time) domain.MessengerChatUserCheck {
	if len(rawJSON) > 0 && !json.Valid(rawJSON) {
		rawJSON = nil
	}
	return domain.MessengerChatUserCheck{
		MessengerKind:   req.MessengerKind,
		ChatRef:         strings.TrimSpace(req.ChatRef),
		MessengerUserID: strings.TrimSpace(req.MessengerUserID),
		UserID:          req.UserID,
		ConnectorID:     req.ConnectorID,
		SubscriptionID:  req.SubscriptionID,
		Source:          strings.TrimSpace(req.Source),
		Status:          state.Status,
		OK:              ok,
		RawJSON:         append([]byte(nil), rawJSON...),
		RawError:        strings.TrimSpace(rawError),
		CheckedAt:       now,
	}
}

func normalizeTelegramStatus(status string) domain.MessengerChatUserStatus {
	switch strings.TrimSpace(status) {
	case "creator":
		return domain.MessengerChatUserStatusOwner
	case "administrator":
		return domain.MessengerChatUserStatusAdministrator
	case "member":
		return domain.MessengerChatUserStatusMember
	case "restricted":
		return domain.MessengerChatUserStatusRestricted
	case "left":
		return domain.MessengerChatUserStatusLeft
	case "kicked":
		return domain.MessengerChatUserStatusBanned
	default:
		return domain.MessengerChatUserStatusUnknown
	}
}
