package services

import (
	"errors"
	"strings"
	"time"

	"github.com/webauto/chatgpt-panel/internal/models"
	"gorm.io/gorm"
)

type AccountService struct {
	db *gorm.DB
}

func NewAccountService() *AccountService {
	return &AccountService{db: models.GetDB()}
}

func (s *AccountService) Create(account *models.Account) error {
	if account.RegisteredAt.IsZero() {
		account.RegisteredAt = time.Now()
	}
	if account.Status == "" {
		account.Status = "pending"
	}
	return s.db.Create(account).Error
}

func (s *AccountService) GetByID(id uint) (*models.Account, error) {
	var account models.Account
	if err := s.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountService) GetByEmail(email string) (*models.Account, error) {
	var account models.Account
	if err := s.db.Where("email = ?", email).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountService) Update(account *models.Account) error {
	return s.db.Save(account).Error
}

func (s *AccountService) Delete(id uint) error {
	return s.db.Delete(&models.Account{}, id).Error
}

func (s *AccountService) BatchDelete(ids []uint) error {
	return s.db.Delete(&models.Account{}, ids).Error
}

func (s *AccountService) UpdateStatus(id uint, status string) error {
	if !isValidStatus(status) {
		return errors.New("invalid status")
	}
	return s.db.Model(&models.Account{}).Where("id = ?", id).Update("status", status).Error
}

func (s *AccountService) BatchUpdateStatus(ids []uint, status string) error {
	if !isValidStatus(status) {
		return errors.New("invalid status")
	}
	return s.db.Model(&models.Account{}).Where("id IN ?", ids).Update("status", status).Error
}

func (s *AccountService) UpdateRefreshToken(id uint, refreshToken string) error {
	return s.db.Model(&models.Account{}).Where("id = ?", id).Updates(map[string]interface{}{
		"refresh_token":      refreshToken,
		"cliproxy_synced":    false,
		"cliproxy_synced_at": nil,
		"updated_at":         time.Now(),
	}).Error
}

func isValidStatus(status string) bool {
	switch status {
	case "pending", "active", "failed", "expired":
		return true
	}
	return false
}

func (s *AccountService) CreateOrUpdate(account *models.Account) error {
	existing, err := s.GetByEmail(account.Email)
	if err == nil && existing != nil {
		account.ID = existing.ID
		account.CreatedAt = existing.CreatedAt
		return s.db.Save(account).Error
	}
	if account.RegisteredAt.IsZero() {
		account.RegisteredAt = time.Now()
	}
	return s.db.Create(account).Error
}

func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func (s *AccountService) List(filter models.AccountFilter) (*models.PaginatedResult, error) {
	var accounts []models.Account
	var total int64

	query := s.db.Model(&models.Account{})

	if filter.Search != "" {
		query = query.Where("email ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Domain != "" {
		query = query.Where("email LIKE ?", "%@"+filter.Domain)
	}
	if filter.HasRT == "yes" {
		query = query.Where("refresh_token IS NOT NULL AND refresh_token != ''")
	} else if filter.HasRT == "no" {
		query = query.Where("refresh_token IS NULL OR refresh_token = ''")
	}
	if filter.DateFrom != "" {
		query = query.Where("created_at >= ?", filter.DateFrom)
	}
	if filter.DateTo != "" {
		query = query.Where("created_at <= ?", filter.DateTo+" 23:59:59")
	}

	query.Count(&total)

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}

	offset := (filter.Page - 1) * filter.PageSize
	order := filter.SortBy + " " + filter.SortDir
	if filter.SortBy == "" {
		order = "created_at desc"
	}

	query.Order(order).Offset(offset).Limit(filter.PageSize).Find(&accounts)

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize > 0 {
		totalPages++
	}

	for i := range accounts {
		if accounts[i].AccessToken != "" {
			if plan := ExtractSubscriptionStatusFromToken(accounts[i].AccessToken); plan != "" && plan != accounts[i].SubscriptionStatus {
				accounts[i].SubscriptionStatus = plan
				_ = s.db.Model(&models.Account{}).Where("id = ?", accounts[i].ID).Update("subscription_status", plan).Error
			}
		}
	}

	return &models.PaginatedResult{
		Data:       accounts,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *AccountService) GetStats() (*models.AccountStats, error) {
	stats := &models.AccountStats{
		ByDomain: make(map[string]int64),
		ByDate:   make([]models.DateCount, 0),
	}

	s.db.Model(&models.Account{}).Count(&stats.Total)
	s.db.Model(&models.Account{}).Where("status = ?", "pending").Count(&stats.Pending)
	s.db.Model(&models.Account{}).Where("status = ?", "active").Count(&stats.Active)
	s.db.Model(&models.Account{}).Where("status = ?", "failed").Count(&stats.Failed)
	s.db.Model(&models.Account{}).Where("status = ?", "expired").Count(&stats.Expired)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := today.AddDate(0, 0, -7)
	monthStart := today.AddDate(0, -1, 0)

	s.db.Model(&models.Account{}).Where("created_at >= ?", today).Count(&stats.TodayCount)
	s.db.Model(&models.Account{}).Where("created_at >= ?", weekStart).Count(&stats.WeekCount)
	s.db.Model(&models.Account{}).Where("created_at >= ?", monthStart).Count(&stats.MonthCount)

	s.db.Model(&models.Account{}).Where("access_token IS NOT NULL AND access_token != ''").Count(&stats.WithToken)
	s.db.Model(&models.Account{}).Where("refresh_token IS NOT NULL AND refresh_token != ''").Count(&stats.WithRefreshToken)
	s.db.Model(&models.Account{}).Where("checkout_url IS NOT NULL AND checkout_url != ''").Count(&stats.WithCheckoutURL)

	var domainCounts []struct {
		Domain string
		Count  int64
	}
	s.db.Model(&models.Account{}).
		Select("SPLIT_PART(email, '@', 2) as domain, COUNT(*) as count").
		Group("domain").Order("count DESC").Limit(20).Find(&domainCounts)

	for _, dc := range domainCounts {
		stats.ByDomain[dc.Domain] = dc.Count
	}

	for i := 6; i >= 0; i-- {
		date := today.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		var count int64
		s.db.Model(&models.Account{}).Where("DATE(created_at) = ?", dateStr).Count(&count)
		stats.ByDate = append(stats.ByDate, models.DateCount{Date: dateStr, Count: count})
	}

	return stats, nil
}
