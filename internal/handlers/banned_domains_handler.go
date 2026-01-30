package handlers

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// BannedDomainsHandler 禁用域名处理器
type BannedDomainsHandler struct {
	filePath string
}

// NewBannedDomainsHandler 创建禁用域名处理器
func NewBannedDomainsHandler() *BannedDomainsHandler {
	// 默认文件路径
	path := os.Getenv("BANNED_DOMAINS_FILE")
	if path == "" {
		path = "banned_email_domains.txt"
	}
	return &BannedDomainsHandler{filePath: path}
}

// List 获取禁用域名列表
// GET /api/v1/banned-domains
func (h *BannedDomainsHandler) List(c *gin.Context) {
	domains, err := h.loadDomains()
	if err != nil {
		// 文件不存在时返回空列表
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{
				"domains": []string{},
				"count":   0,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"domains": domains,
		"count":   len(domains),
	})
}

// GetRaw 获取原始文件内容（纯文本）
// GET /api/v1/banned-domains/raw
func (h *BannedDomainsHandler) GetRaw(c *gin.Context) {
	absPath, _ := filepath.Abs(h.filePath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.String(http.StatusOK, "")
			return
		}
		c.String(http.StatusInternalServerError, "Error reading file: %v", err)
		return
	}
	c.String(http.StatusOK, string(data))
}

// Append 追加禁用域名
// POST /api/v1/banned-domains
func (h *BannedDomainsHandler) Append(c *gin.Context) {
	var req struct {
		Domains []string `json:"domains" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, _ := h.loadDomains()
	existingMap := make(map[string]bool)
	for _, d := range existing {
		existingMap[strings.ToLower(d)] = true
	}

	var added []string
	absPath, _ := filepath.Abs(h.filePath)
	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()

	for _, domain := range req.Domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" || strings.HasPrefix(domain, "#") {
			continue
		}
		if existingMap[domain] {
			continue
		}
		if _, err := f.WriteString(domain + "\n"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		added = append(added, domain)
		existingMap[domain] = true
	}

	c.JSON(http.StatusOK, gin.H{
		"added":   added,
		"count":   len(added),
		"message": "Domains appended successfully",
	})
}

// loadDomains 加载域名列表
func (h *BannedDomainsHandler) loadDomains() ([]string, error) {
	absPath, _ := filepath.Abs(h.filePath)
	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var domains []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domains = append(domains, strings.ToLower(line))
	}
	return domains, scanner.Err()
}

