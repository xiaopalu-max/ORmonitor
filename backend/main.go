package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sessionTTL = 24 * time.Hour

type database struct {
	Keys           []map[string]any  `json:"keys"`
	Settings       map[string]any    `json:"settings"`
	Auth           map[string]string `json:"auth"`
	BalanceHistory []map[string]any  `json:"balanceHistory"`
}

type store struct {
	mu   sync.RWMutex
	path string
	data database
}

type session struct {
	Username  string    `json:"username"`
	LoginTime time.Time `json:"loginTime"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type server struct {
	store          *store
	sessions       map[string]session
	sessionsMu     sync.Mutex
	staticHandlers []http.Handler
	httpClient     *http.Client
	notifyMu       sync.Mutex
	notifyCancel   context.CancelFunc
}

func main() {
	port := env("PORT", "3000")
	dbPath := env("DATA_PATH", filepath.Join("..", "data", "db.json"))

	st, err := newStore(dbPath)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	s := &server{
		store:    st,
		sessions: make(map[string]session),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Preserve legacy webhook behavior.
			},
		},
	}
	s.staticHandlers = discoverStaticHandlers()

	if err := s.startNotifyTask(false); err != nil {
		log.Printf("start notification task: %v", err)
	}

	addr := ":" + port
	log.Printf("service started: http://localhost:%s", port)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}

func newStore(path string) (*store, error) {
	st := &store{path: path}
	if err := st.load(); err != nil {
		return nil, err
	}
	if err := st.writeLocked(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		s.data = defaultDatabase()
		return nil
	}
	if len(bytes.TrimSpace(b)) == 0 {
		s.data = defaultDatabase()
		return nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return err
	}
	normalizeDatabase(&s.data)
	return nil
}

func (s *store) read(fn func(*database) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fn(&s.data)
}

func (s *store) update(fn func(*database) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	normalizeDatabase(&s.data)
	return s.writeLocked()
}

func (s *store) writeLocked() error {
	normalizeDatabase(&s.data)
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func defaultDatabase() database {
	return database{
		Keys:           []map[string]any{},
		Settings:       map[string]any{},
		Auth:           map[string]string{"username": "admin", "password": "admin123"},
		BalanceHistory: []map[string]any{},
	}
}

func normalizeDatabase(db *database) {
	if db.Keys == nil {
		db.Keys = []map[string]any{}
	}
	if db.Settings == nil {
		db.Settings = map[string]any{}
	}
	if db.Auth == nil {
		db.Auth = map[string]string{"username": "admin", "password": "admin123"}
	}
	if db.Auth["username"] == "" {
		db.Auth["username"] = "admin"
	}
	if db.Auth["password"] == "" {
		db.Auth["password"] = "admin123"
	}
	if db.BalanceHistory == nil {
		db.BalanceHistory = []map[string]any{}
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.withCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.handleAPI(w, r)
		return
	}
	s.serveStatic(w, r)
}

func (s *server) withCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (s *server) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "api/auth/login":
		s.handleLogin(w, r)
	case r.Method == http.MethodGet && path == "api/auth/check":
		s.handleAuthCheck(w, r)
	case r.Method == http.MethodPost && path == "api/auth/logout":
		s.handleLogout(w, r)
	case r.Method == http.MethodPost && path == "api/auth/update":
		s.requireAuth(w, r, s.handleAuthUpdate)
	case r.Method == http.MethodGet && path == "api/keys":
		s.requireAuth(w, r, s.handleKeysList)
	case r.Method == http.MethodPost && path == "api/keys":
		s.requireAuth(w, r, s.handleKeyCreate)
	case len(parts) == 3 && parts[0] == "api" && parts[1] == "keys" && r.Method == http.MethodPut:
		s.requireAuth(w, r, func(w http.ResponseWriter, r *http.Request) { s.handleKeyUpdate(w, r, parts[2]) })
	case len(parts) == 3 && parts[0] == "api" && parts[1] == "keys" && r.Method == http.MethodDelete:
		s.requireAuth(w, r, func(w http.ResponseWriter, r *http.Request) { s.handleKeyDelete(w, r, parts[2]) })
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "keys" && parts[3] == "archive" && r.Method == http.MethodPost:
		s.requireAuth(w, r, func(w http.ResponseWriter, r *http.Request) { s.handleKeyArchive(w, r, parts[2], true) })
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "keys" && parts[3] == "archive" && r.Method == http.MethodDelete:
		s.requireAuth(w, r, func(w http.ResponseWriter, r *http.Request) { s.handleKeyArchive(w, r, parts[2], false) })
	case r.Method == http.MethodGet && path == "api/balance-history":
		s.requireAuth(w, r, s.handleBalanceHistoryList)
	case r.Method == http.MethodPost && path == "api/balance-history":
		s.requireAuth(w, r, s.handleBalanceHistoryCreate)
	case r.Method == http.MethodGet && path == "api/settings":
		s.requireAuth(w, r, s.handleSettingsGet)
	case r.Method == http.MethodPost && path == "api/settings":
		s.requireAuth(w, r, s.handleSettingsSave)
	case r.Method == http.MethodPost && path == "api/settings/template":
		s.requireAuth(w, r, s.handleSettingsTemplateSave)
	case r.Method == http.MethodPost && path == "api/settings/price":
		s.requireAuth(w, r, s.handleSettingsPriceSave)
	case r.Method == http.MethodPost && path == "api/test-channel":
		s.requireAuth(w, r, s.handleTestChannel)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (s *server) requireAuth(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	sessionID := getSessionID(r)
	if sessionID == "" || !s.sessionValid(sessionID) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录", "loggedIn": false})
		return
	}
	next(w, r)
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	username := stringValue(body["username"])
	password := stringValue(body["password"])

	var ok bool
	if err := s.store.read(func(db *database) error {
		ok = username == db.Auth["username"] && password == db.Auth["password"]
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "账号或密码错误"})
		return
	}

	sessionID, err := randomID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "生成会话失败"})
		return
	}
	now := time.Now()
	s.sessionsMu.Lock()
	s.sessions[sessionID] = session{Username: username, LoginTime: now, ExpiresAt: now.Add(sessionTTL)}
	s.sessionsMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "sessionId": sessionID})
}

func (s *server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"loggedIn": s.sessionValid(getSessionID(r))})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	if sessionID != "" {
		s.sessionsMu.Lock()
		delete(s.sessions, sessionID)
		s.sessionsMu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleAuthUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	username := stringValue(body["username"])
	password := stringValue(body["password"])
	if username == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "账号和密码不能为空"})
		return
	}
	if err := s.store.update(func(db *database) error {
		db.Auth = map[string]string{"username": username, "password": password}
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleKeysList(w http.ResponseWriter, _ *http.Request) {
	var keys []map[string]any
	if err := s.store.read(func(db *database) error {
		keys = cloneSlice(db.Keys)
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *server) handleKeyCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	name := stringValue(body["name"])
	key := stringValue(body["key"])
	if name == "" || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少参数"})
		return
	}

	newKey := map[string]any{
		"id":         mustRandomID(),
		"name":       name,
		"key":        key,
		"quota":      numberValue(body["quota"]),
		"group":      defaultString(stringValue(body["group"]), "默认分组"),
		"createTime": time.Now().Format(time.RFC3339Nano),
	}
	copyOptional(newKey, body, "exhaustedThreshold")
	copyOptional(newKey, body, "warningThreshold")
	copyOptional(newKey, body, "purchaseRate")
	copyOptional(newKey, body, "sellRate")

	if err := s.store.update(func(db *database) error {
		db.Keys = append(db.Keys, newKey)
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, newKey)
}

func (s *server) handleKeyUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	name := stringValue(body["name"])
	key := stringValue(body["key"])
	if name == "" || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少参数"})
		return
	}

	var updated map[string]any
	err := s.store.update(func(db *database) error {
		for i := range db.Keys {
			if stringValue(db.Keys[i]["id"]) != id {
				continue
			}
			next := cloneMap(db.Keys[i])
			next["name"] = name
			next["key"] = key
			next["quota"] = numberValue(body["quota"])
			next["group"] = defaultString(stringValue(body["group"]), "默认分组")
			applyNullableOptional(next, body, "exhaustedThreshold")
			applyNullableOptional(next, body, "warningThreshold")
			applyNullableOptional(next, body, "purchaseRate")
			applyNullableOptional(next, body, "sellRate")
			db.Keys[i] = next
			updated = cloneMap(next)
			return nil
		}
		return errNotFound
	})
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "密钥不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) handleKeyDelete(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.store.update(func(db *database) error {
		next := db.Keys[:0]
		for _, item := range db.Keys {
			if stringValue(item["id"]) != id {
				next = append(next, item)
			}
		}
		db.Keys = next
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleKeyArchive(w http.ResponseWriter, _ *http.Request, id string, archive bool) {
	err := s.store.update(func(db *database) error {
		for i := range db.Keys {
			if stringValue(db.Keys[i]["id"]) != id {
				continue
			}
			if archive {
				db.Keys[i]["archived"] = true
				db.Keys[i]["archivedTime"] = time.Now().Format(time.RFC3339Nano)
			} else {
				delete(db.Keys[i], "archived")
				delete(db.Keys[i], "archivedTime")
			}
			return nil
		}
		return errNotFound
	})
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "密钥不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleBalanceHistoryCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if _, ok := body["keyId"]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少参数"})
		return
	}
	if _, ok := body["balance"]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少参数"})
		return
	}

	now := time.Now()
	entry := map[string]any{
		"id":        mustRandomID(),
		"keyId":     defaultString(stringValue(body["keyId"]), "all"),
		"balance":   numberValue(body["balance"]),
		"usage":     numberValue(body["usage"]),
		"remaining": numberValue(body["remaining"]),
		"timestamp": now.Format(time.RFC3339Nano),
		"date":      now.Format("2006-01-02"),
	}
	cutoff := now.AddDate(0, 0, -90)

	if err := s.store.update(func(db *database) error {
		db.BalanceHistory = append(db.BalanceHistory, entry)
		filtered := db.BalanceHistory[:0]
		for _, item := range db.BalanceHistory {
			ts, err := time.Parse(time.RFC3339Nano, stringValue(item["timestamp"]))
			if err == nil && ts.After(cutoff) {
				filtered = append(filtered, item)
			}
		}
		db.BalanceHistory = filtered
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "entry": entry})
}

func (s *server) handleBalanceHistoryList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyID := q.Get("keyId")
	startDate := q.Get("startDate")
	endDate := q.Get("endDate")

	var history []map[string]any
	if err := s.store.read(func(db *database) error {
		for _, entry := range db.BalanceHistory {
			entryKeyID := stringValue(entry["keyId"])
			if keyID != "" && keyID != "all" && entryKeyID != keyID {
				continue
			}
			if keyID == "all" && entryKeyID != "all" {
				continue
			}
			date := stringValue(entry["date"])
			if startDate != "" && date < startDate {
				continue
			}
			if endDate != "" && date > endDate {
				continue
			}
			history = append(history, cloneMap(entry))
		}
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	sort.SliceStable(history, func(i, j int) bool {
		return stringValue(history[i]["timestamp"]) < stringValue(history[j]["timestamp"])
	})
	writeJSON(w, http.StatusOK, history)
}

func (s *server) handleSettingsGet(w http.ResponseWriter, _ *http.Request) {
	var settings map[string]any
	if err := s.store.read(func(db *database) error {
		settings = cloneMap(db.Settings)
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	fields := map[string]any{
		"webhookUrl":         "",
		"enableNotify":       false,
		"notifyInterval":     float64(60),
		"purchaseRate":       float64(3.5),
		"sellRate":           float64(4.0),
		"exhaustedThreshold": float64(2),
		"warningThreshold":   float64(20),
		"notifyChannels":     map[string]any{},
	}

	var settings map[string]any
	if err := s.store.update(func(db *database) error {
		next := cloneMap(db.Settings)
		for name, fallback := range fields {
			if v, ok := body[name]; ok {
				next[name] = v
			} else if _, exists := next[name]; !exists {
				next[name] = fallback
			}
		}
		delete(next, "notifyTime")
		db.Settings = next
		settings = cloneMap(next)
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.startNotifyTask(false); err != nil {
		log.Printf("restart notification task: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "settings": settings})
}

func (s *server) handleSettingsTemplateSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.store.update(func(db *database) error {
		next := cloneMap(db.Settings)
		if v, ok := body["notifyTemplate"]; ok {
			next["notifyTemplate"] = v
		} else if _, exists := next["notifyTemplate"]; !exists {
			next["notifyTemplate"] = ""
		}
		if v, ok := body["keyTemplate"]; ok {
			next["keyTemplate"] = v
		} else if _, exists := next["keyTemplate"]; !exists {
			next["keyTemplate"] = ""
		}
		db.Settings = next
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleSettingsPriceSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.store.update(func(db *database) error {
		next := cloneMap(db.Settings)
		purchaseRate := numberValue(body["purchaseRate"])
		sellRate := numberValue(body["sellRate"])
		if purchaseRate == 0 {
			purchaseRate = 3.5
		}
		if sellRate == 0 {
			sellRate = 4.0
		}
		next["purchaseRate"] = purchaseRate
		next["sellRate"] = sellRate
		db.Settings = next
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请提供渠道类型"})
		return
	}
	var config map[string]any
	if err := readJSON(r, &config); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	message := fmt.Sprintf("**测试消息**\n\n这是一条测试消息，用于验证%s配置是否正确。\n\n发送时间: %s", channelName(channel), time.Now().Format("2006-01-02 15:04:05"))
	var err error
	switch channel {
	case "wechat":
		err = s.sendWebhook(stringValue(config["webhook"]), map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"content": message},
		}, "errcode", "errmsg")
	case "dingtalk":
		err = s.sendWebhook(stringValue(config["webhook"]), map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "OpenRouter余额通知",
				"text":  message,
			},
		}, "errcode", "errmsg")
	case "feishu":
		err = s.sendWebhook(stringValue(config["webhook"]), feishuMessage(message), "code", "msg")
	case "email":
		err = sendEmail(config, "测试消息", message)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不支持的渠道类型"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) startNotifyTask(immediateFirst bool) error {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()

	if s.notifyCancel != nil {
		s.notifyCancel()
		s.notifyCancel = nil
	}

	var settings map[string]any
	if err := s.store.read(func(db *database) error {
		settings = cloneMap(db.Settings)
		return nil
	}); err != nil {
		return err
	}
	channels := asMap(settings["notifyChannels"])
	if !boolValue(settings["enableNotify"]) || !hasEnabledChannel(channels) {
		log.Print("定时发送未启用或未配置通知渠道")
		return nil
	}

	intervalMinutes := numberValue(settings["notifyInterval"])
	if intervalMinutes <= 0 {
		intervalMinutes = 60
	}
	interval := time.Duration(intervalMinutes * float64(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	s.notifyCancel = cancel

	go func() {
		if immediateFirst {
			s.sendScheduledNotification(ctx)
		}
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.sendScheduledNotification(ctx)
				timer.Reset(interval)
			}
		}
	}()
	log.Printf("定时发送已启动，将在 %.0f 分钟后首次发送，之后每 %.0f 分钟发送一次", intervalMinutes, intervalMinutes)
	return nil
}

func (s *server) sendScheduledNotification(ctx context.Context) {
	message, err := s.generateNotifyMessage(ctx)
	if err != nil {
		log.Printf("生成通知失败: %v", err)
		return
	}
	if message.Content == "" {
		return
	}
	var settings map[string]any
	if err := s.store.read(func(db *database) error {
		settings = cloneMap(db.Settings)
		return nil
	}); err != nil {
		log.Printf("读取通知配置失败: %v", err)
		return
	}
	channels := asMap(settings["notifyChannels"])
	sendIf := func(name string, fn func(map[string]any) error) {
		ch := asMap(channels[name])
		if boolValue(ch["enabled"]) {
			if err := fn(ch); err != nil {
				log.Printf("%s消息发送失败: %v", channelName(name), err)
			} else {
				log.Printf("%s消息发送成功", channelName(name))
			}
		}
	}
	sendIf("wechat", func(ch map[string]any) error {
		return s.sendWebhook(stringValue(ch["webhook"]), map[string]any{"msgtype": "markdown", "markdown": map[string]any{"content": message.Content}}, "errcode", "errmsg")
	})
	sendIf("dingtalk", func(ch map[string]any) error {
		return s.sendWebhook(stringValue(ch["webhook"]), map[string]any{"msgtype": "markdown", "markdown": map[string]any{"title": "OpenRouter余额通知", "text": message.PlainText}}, "errcode", "errmsg")
	})
	sendIf("feishu", func(ch map[string]any) error {
		return s.sendWebhook(stringValue(ch["webhook"]), feishuMessage(message.PlainText), "code", "msg")
	})
	sendIf("email", func(ch map[string]any) error {
		return sendEmail(ch, "OpenRouter余额通知", message.PlainText)
	})
}

type notifyMessage struct {
	Content   string
	PlainText string
}

func (s *server) generateNotifyMessage(ctx context.Context) (notifyMessage, error) {
	var keys []map[string]any
	var settings map[string]any
	if err := s.store.read(func(db *database) error {
		settings = cloneMap(db.Settings)
		for _, key := range db.Keys {
			group := defaultString(stringValue(key["group"]), "默认分组")
			if !boolValue(key["archived"]) && group != "失效分组" {
				keys = append(keys, cloneMap(key))
			}
		}
		return nil
	}); err != nil {
		return notifyMessage{}, err
	}
	if len(keys) == 0 {
		return notifyMessage{}, nil
	}

	var details []keyDetail
	var totalUsage, totalRemaining, totalDaily float64
	for _, key := range keys {
		detail := keyDetail{Name: stringValue(key["name"]), Key: stringValue(key["key"]), Quota: numberValue(key["quota"])}
		usage, daily, err := s.fetchOpenRouterUsage(ctx, detail.Key)
		if err != nil {
			detail.Status = "❌ 错误"
			detail.Error = err.Error()
			details = append(details, detail)
			continue
		}
		detail.Usage = usage
		detail.Daily = daily
		detail.Remaining = math.Max(0, detail.Quota-usage)
		purchaseRate := defaultNumber(key["purchaseRate"], defaultNumber(settings["purchaseRate"], 3.5))
		sellRate := defaultNumber(key["sellRate"], defaultNumber(settings["sellRate"], 4.0))
		exhaustedThreshold := defaultNumber(key["exhaustedThreshold"], defaultNumber(settings["exhaustedThreshold"], 2))
		warningThreshold := defaultNumber(key["warningThreshold"], defaultNumber(settings["warningThreshold"], 20))

		if detail.Quota > 0 && detail.Remaining < exhaustedThreshold {
			if err := s.archiveKey(stringValue(key["id"])); err != nil {
				log.Printf("自动归档密钥失败 %s: %v", detail.Name, err)
			}
			continue
		}
		if detail.Quota > 0 {
			detail.RemainingPercent = detail.Remaining / detail.Quota * 100
		}
		detail.TotalProfit = usage * (sellRate - purchaseRate)
		detail.DailyProfit = daily * (sellRate - purchaseRate)
		detail.Status = "✅ 健康"
		if detail.Remaining < exhaustedThreshold {
			detail.Status = "❌ 耗尽"
		} else if detail.Remaining < warningThreshold {
			detail.Status = "⚠️ 警告"
		}

		totalUsage += usage
		totalDaily += daily
		totalRemaining += detail.Remaining
		details = append(details, detail)
	}

	purchaseRate := defaultNumber(settings["purchaseRate"], 3.5)
	sellRate := defaultNumber(settings["sellRate"], 4.0)
	var totalQuota, totalProfit, totalDailyProfit float64
	for _, d := range details {
		if d.Error == "" {
			totalQuota += d.Quota
			totalProfit += d.TotalProfit
			totalDailyProfit += d.DailyProfit
		}
	}
	totalRemainingPercent := 0.0
	if totalQuota > 0 {
		totalRemainingPercent = totalRemaining / totalQuota * 100
	}

	timeStr := time.Now().Format("01-02 15:04")
	keyRows := renderKeyRows(details, stringValue(settings["keyTemplate"]), purchaseRate)
	template := stringValue(settings["notifyTemplate"])
	var content string
	if template != "" {
		replacements := map[string]string{
			"{{date}}":                  timeStr,
			"{{totalQuota}}":            fmt.Sprintf("%.2f", totalQuota),
			"{{totalUsage}}":            fmt.Sprintf("%.2f", totalUsage),
			"{{totalRemaining}}":        fmt.Sprintf("%.2f", totalRemaining),
			"{{totalRemainingCNY}}":     fmt.Sprintf("%.2f", totalRemaining*purchaseRate),
			"{{totalRemainingPercent}}": fmt.Sprintf("%.0f", totalRemainingPercent),
			"{{totalDaily}}":            fmt.Sprintf("%.2f", totalDaily),
			"{{totalProfit}}":           fmt.Sprintf("%.2f", totalProfit),
			"{{totalDailyProfit}}":      fmt.Sprintf("%.2f", totalDailyProfit),
			"{{purchaseRate}}":          fmt.Sprintf("%g", purchaseRate),
			"{{sellRate}}":              fmt.Sprintf("%g", sellRate),
			"{{keys}}":                  keyRows,
		}
		content = replaceAll(template, replacements)
	} else {
		content = fmt.Sprintf("📊 OpenRouter · %s    今日收购价格： %g¥    出售价格： %g¥\n━━━━━━━━━━━━━━━━━━━━\n\n%s━━━━━━━━━━━━━━━━━━━━\n💰 合计\n    L 额度 [$%.2f]\n    L 剩余 [$%.2f] x %g = [¥%.2f]  百分比 ≈ (%.0f%%)\n    L 总消耗 [$%.2f]  总利润：%.2f¥\n    L 今日消耗 [$%.2f]  今日利润：%.2f¥\n",
			timeStr, purchaseRate, sellRate, keyRows, totalQuota, totalRemaining, purchaseRate, totalRemaining*purchaseRate, totalRemainingPercent, totalUsage, totalProfit, totalDaily, totalDailyProfit)
	}
	return notifyMessage{Content: content, PlainText: content}, nil
}

func (s *server) fetchOpenRouterUsage(ctx context.Context, apiKey string) (float64, float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, 0, err
	}
	if body["error"] != nil {
		return 0, 0, fmt.Errorf("%v", body["error"])
	}
	data := asMap(body["data"])
	return numberValue(data["usage"]), numberValue(data["usage_daily"]), nil
}

func (s *server) archiveKey(id string) error {
	return s.store.update(func(db *database) error {
		for _, key := range db.Keys {
			if stringValue(key["id"]) == id {
				key["archived"] = true
				key["archivedTime"] = time.Now().Format(time.RFC3339Nano)
				return nil
			}
		}
		return nil
	})
}

func renderKeyRows(details []keyDetail, template string, purchaseRate float64) string {
	var b strings.Builder
	for i, d := range details {
		if d.Error != "" {
			row := defaultString(template, "{{index}}. {{name}} --> ❌ 错误: {{error}}")
			b.WriteString(replaceAll(row, map[string]string{
				"{{index}}": strconv.Itoa(i + 1),
				"{{name}}":  d.Name,
				"{{error}}": d.Error,
			}))
		} else {
			row := defaultString(template, "{{index}}. {{name}} --> 余额 [${{remaining}}] x {{purchaseRate}} = [¥{{remainingCNY}}]  百分比 ≈ ({{remainingPercent}}%)\n    L 密钥--> [ {{key}} ]\n    L 额度--> [${{quota}}]\n    L 总消耗 -${{usage}}  (总利润：{{profit}}¥)\n    L 今日消耗 -${{daily}}  (今日利润：{{dailyProfit}}¥)\n    L 状态：{{status}}")
			b.WriteString(replaceAll(row, map[string]string{
				"{{index}}":            strconv.Itoa(i + 1),
				"{{name}}":             d.Name,
				"{{remaining}}":        fmt.Sprintf("%.2f", d.Remaining),
				"{{remainingCNY}}":     fmt.Sprintf("%.2f", d.Remaining*purchaseRate),
				"{{remainingPercent}}": fmt.Sprintf("%.0f", d.RemainingPercent),
				"{{key}}":              d.Key,
				"{{quota}}":            fmt.Sprintf("%.2f", d.Quota),
				"{{usage}}":            fmt.Sprintf("%.2f", d.Usage),
				"{{profit}}":           fmt.Sprintf("%.2f", d.TotalProfit),
				"{{daily}}":            fmt.Sprintf("%.2f", d.Daily),
				"{{dailyProfit}}":      fmt.Sprintf("%.2f", d.DailyProfit),
				"{{status}}":           d.Status,
				"{{purchaseRate}}":     fmt.Sprintf("%g", purchaseRate),
			}))
		}
		if i < len(details)-1 {
			b.WriteString("\n")
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

type keyDetail struct {
	Name, Key, Status, Error                   string
	Quota, Usage, Remaining, Daily             float64
	RemainingPercent, TotalProfit, DailyProfit float64
}

func (s *server) sendWebhook(webhook string, payload map[string]any, codeField, messageField string) error {
	if webhook == "" {
		return errors.New("Webhook URL不能为空")
	}
	if _, err := url.ParseRequestURI(webhook); err != nil {
		return fmt.Errorf("Webhook URL格式不正确: %w", err)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Post(webhook, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result map[string]any
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if len(body) > 0 {
		_ = json.Unmarshal(body, &result)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if code, ok := result[codeField]; ok && numberValue(code) != 0 {
		msg := stringValue(result[messageField])
		if msg == "" {
			msg = string(body)
		}
		return errors.New(defaultString(msg, "发送失败"))
	}
	return nil
}

func sendEmail(config map[string]any, subject, content string) error {
	host := stringValue(config["host"])
	from := stringValue(config["from"])
	password := stringValue(config["password"])
	to := stringValue(config["to"])
	port := int(defaultNumber(config["port"], 587))
	if host == "" || from == "" || to == "" {
		return errors.New("邮件配置不完整")
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	message := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Content-Type: text/html; charset=utf-8",
		"",
		strings.ReplaceAll(html.EscapeString(content), "\n", "<br>"),
	}, "\r\n")

	auth := smtp.PlainAuth("", from, password, host)
	if port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return err
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, message); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}
	return sendMailStartTLS(addr, host, auth, from, to, []byte(message))
}

func sendMailStartTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: true}); err != nil {
			return err
		}
	}
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func feishuMessage(content string) map[string]any {
	return map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"title":    map[string]any{"tag": "plain_text", "content": "OpenRouter余额通知"},
				"template": "blue",
			},
			"elements": []map[string]any{{
				"tag":  "div",
				"text": map[string]any{"tag": "lark_md", "content": content},
			}},
		},
	}
}

func (s *server) sessionValid(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	now := time.Now()
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	sess, ok := s.sessions[sessionID]
	return ok && now.Before(sess.ExpiresAt)
}

func discoverStaticHandlers() []http.Handler {
	candidates := []string{
		env("STATIC_DIR", ""),
		filepath.Join("..", "frontend", "dist"),
		filepath.Join("..", "public"),
		"public",
	}
	var handlers []http.Handler
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			handlers = append(handlers, spaFileServer(dir))
		}
	}
	return handlers
}

func spaFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func (s *server) serveStatic(w http.ResponseWriter, r *http.Request) {
	for _, handler := range s.staticHandlers {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		handler.ServeHTTP(rec, r)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

var errNotFound = errors.New("not found")

func getSessionID(r *http.Request) string {
	if id := r.Header.Get("X-Session-ID"); id != "" {
		return id
	}
	return r.URL.Query().Get("sessionId")
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write response: %v", err)
	}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func mustRandomID() string {
	id, err := randomID()
	if err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return id
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func numberValue(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(stringValue(x), 64)
		return f
	}
}

func defaultNumber(v any, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return numberValue(v)
}

func boolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	default:
		return numberValue(v) != 0
	}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSlice(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		out = append(out, cloneMap(item))
	}
	return out
}

func copyOptional(dst, src map[string]any, key string) {
	if v, ok := src[key]; ok {
		dst[key] = v
	}
}

func applyNullableOptional(dst, src map[string]any, key string) {
	v, ok := src[key]
	if !ok {
		return
	}
	if v == nil {
		delete(dst, key)
		return
	}
	dst[key] = v
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func channelName(channel string) string {
	switch channel {
	case "wechat":
		return "企业微信"
	case "dingtalk":
		return "钉钉"
	case "feishu":
		return "飞书"
	case "email":
		return "邮件"
	default:
		return channel
	}
}

func hasEnabledChannel(channels map[string]any) bool {
	for _, raw := range channels {
		if boolValue(asMap(raw)["enabled"]) {
			return true
		}
	}
	return false
}

func replaceAll(input string, replacements map[string]string) string {
	for old, newValue := range replacements {
		input = strings.ReplaceAll(input, old, newValue)
	}
	return input
}

func init() {
	// Force smtp package to keep net/smtp's textproto scanner from being pruned in
	// older static-analysis setups that inspect direct symbol references.
	_ = bufio.ErrInvalidUnreadByte
}
