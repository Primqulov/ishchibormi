// Package posting implements the Telegram listing form. Every public write
// goes through the same API as the mobile/web clients.
package posting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Profile struct {
	ID        string `json:"id"`
	Phone     string `json:"phone"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Region    string `json:"region"`
	District  string `json:"district"`
}
type Session struct {
	AccessToken string  `json:"accessToken"`
	User        Profile `json:"user"`
}
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Listing struct {
	ID string `json:"id"`
}

type API interface {
	WorkerAPI
	Nearby(context.Context, float64, float64, int, ...SearchFilter) (NearbyPage, error)
	JobAlert(context.Context, Session) (*JobAlert, error)
	SaveJobAlert(context.Context, Session, JobAlert) (*JobAlert, error)
	DeleteJobAlert(context.Context, Session) error
	Login(context.Context, int64, string) (Session, error)
	Categories(context.Context) ([]Category, error)
	SaveProfile(context.Context, Session, Profile) error
	Upload(context.Context, Session, []byte) (string, error)
	RemoveUpload(context.Context, Session, string) error
	Publish(context.Context, Session, string, Form) (Listing, error)
}

type NearbyJob struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	CategoryName    string  `json:"categoryName"`
	Region          string  `json:"region"`
	District        string  `json:"district"`
	Lat             float64 `json:"lat"`
	Lng             float64 `json:"lng"`
	DistanceMeters  float64 `json:"distanceMeters"`
	WorkersNeeded   int     `json:"workersNeeded"`
	AcceptedCount   int     `json:"acceptedCount"`
	PricingType     string  `json:"pricingType"`
	PerWorkerAmount int64   `json:"perWorkerAmount"`
	StartDate       string  `json:"startDate"`
	WorkTimeFrom    string  `json:"workTimeFrom"`
	Gender          string  `json:"gender"`
}
type NearbyPage struct {
	Items []NearbyJob `json:"items"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
	Total int         `json:"total"`
}

type SearchFilter struct{ Day, CategoryID string }

func (c *Client) Nearby(ctx context.Context, lat, lng float64, page int, filters ...SearchFilter) (NearbyPage, error) {
	query := url.Values{"lat": {strconv.FormatFloat(lat, 'f', -1, 64)}, "lng": {strconv.FormatFloat(lng, 'f', -1, 64)}, "page": {strconv.Itoa(page)}, "limit": {"5"}}
	if len(filters) > 0 {
		if filters[0].Day != "" {
			query.Set("day", filters[0].Day)
		}
		if filters[0].CategoryID != "" {
			query.Set("categoryId", filters[0].CategoryID)
		}
	}
	var out NearbyPage
	err := c.request(ctx, "GET", "/api/elons/nearby?"+query.Encode(), "", "", "application/json", nil, &out, false)
	return out, err
}

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details struct {
		Warning string `json:"warning"`
	} `json:"details"`
}

func (e *APIError) Error() string { return e.Message }
func errorCode(err error) string {
	var e *APIError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func definiteRejection(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status >= 400 && e.Status < 500 && e.Status != 408
}
func userError(err error) string {
	var e *APIError
	if !errors.As(err, &e) || e.Status >= 500 {
		return "Xizmat bilan bog'lanib bo'lmadi. Ma'lumotlar saqlangan, qayta urinib ko'ring."
	}
	if e.Status == 429 {
		return "So'rovlar ko'payib ketdi. Birozdan keyin qayta urinib ko'ring."
	}
	if e.Code == "account_disabled" || e.Code == "account_blocked" {
		return "Hisobingiz bloklangan. Qo'llab-quvvatlash xizmatiga murojaat qiling."
	}
	text := e.Message
	if e.Details.Warning != "" {
		text += "\n\n" + e.Details.Warning
	}
	return text
}

type Client struct {
	base, secret string
	http         *http.Client
}

func NewClient(base, secret string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("BOT_API_BASE_URL must be an http(s) origin")
	}
	if len(secret) < 32 || strings.HasPrefix(secret, "change-me") {
		return nil, errors.New("BOT_SHARED_SECRET must be at least 32 characters and match the API")
	}
	return &Client{base: strings.TrimRight(base, "/"), secret: secret, http: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) request(ctx context.Context, method, path, token, key, contentType string, body []byte, result any, signed bool) error {
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", contentType)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if signed {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(c.secret))
		mac.Write([]byte(ts + "\n" + method + "\n" + path + "\n"))
		mac.Write(body)
		r.Header.Set("X-Bot-Timestamp", ts)
		r.Header.Set("X-Bot-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return errors.New("API connection failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return errors.New("API response interrupted")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var envelope struct {
			Error APIError `json:"error"`
		}
		_ = json.Unmarshal(data, &envelope)
		envelope.Error.Status = resp.StatusCode
		if envelope.Error.Message == "" {
			envelope.Error.Message = "So'rov bajarilmadi. Qayta urinib ko'ring."
		}
		return &envelope.Error
	}
	if result != nil && json.Unmarshal(data, result) != nil {
		return errors.New("invalid API response")
	}
	return nil
}
func (c *Client) Login(ctx context.Context, tgID int64, phone string) (Session, error) {
	input := map[string]any{"telegramId": tgID}
	if phone != "" {
		input["phone"] = phone
		input["contactUserId"] = tgID
	}
	b, _ := json.Marshal(input)
	var out Session
	err := c.request(ctx, "POST", "/api/auth/bot/session", "", "", "application/json", b, &out, true)
	if err == nil && (out.AccessToken == "" || out.User.ID == "") {
		err = errors.New("invalid bot session")
	}
	return out, err
}
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var out []Category
	err := c.request(ctx, "GET", "/api/categories", "", "", "application/json", nil, &out, false)
	return out, err
}
func (c *Client) SaveProfile(ctx context.Context, s Session, p Profile) error {
	b, _ := json.Marshal(map[string]string{"firstName": p.FirstName, "lastName": p.LastName, "region": p.Region, "district": p.District})
	return c.request(ctx, "PATCH", "/api/me", s.AccessToken, "", "application/json", b, nil, false)
}
func (c *Client) Upload(ctx context.Context, s Session, data []byte) (string, error) {
	var buf bytes.Buffer
	m := multipart.NewWriter(&buf)
	f, err := m.CreateFormFile("file", "telegram-photo.jpg")
	if err != nil {
		return "", err
	}
	if _, err = f.Write(data); err != nil {
		return "", err
	}
	if err = m.Close(); err != nil {
		return "", err
	}
	var out struct {
		URL string `json:"url"`
	}
	err = c.request(ctx, "POST", "/api/uploads?kind=elon", s.AccessToken, "", m.FormDataContentType(), buf.Bytes(), &out, false)
	if err == nil && out.URL == "" {
		err = fmt.Errorf("upload URL missing")
	}
	return out.URL, err
}
func (c *Client) RemoveUpload(ctx context.Context, s Session, raw string) error {
	return c.request(ctx, "DELETE", "/api/uploads?url="+url.QueryEscape(raw), s.AccessToken, "", "application/json", nil, nil, false)
}
func (c *Client) Publish(ctx context.Context, s Session, key string, form Form) (Listing, error) {
	b, _ := json.Marshal(form)
	var out Listing
	err := c.request(ctx, "POST", "/api/elons", s.AccessToken, key, "application/json", b, &out, true)
	if err == nil && out.ID == "" {
		err = errors.New("listing ID missing")
	}
	return out, err
}
