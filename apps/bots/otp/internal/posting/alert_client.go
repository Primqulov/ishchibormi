package posting

import (
	"context"
	"encoding/json"
)

// JobAlert — foydalanuvchining «ish signali» obunasi (backend:
// internal/jobalert). Bitta odamda bitta signal bo'ladi, shuning uchun
// yozish — oddiy almashtirish, ro'yxat emas.
type JobAlert struct {
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	RadiusM      int     `json:"radiusM"`
	CategoryID   string  `json:"categoryId,omitempty"`
	CategoryName string  `json:"categoryName,omitempty"`
	PlaceName    string  `json:"placeName,omitempty"`
}

type alertEnvelope struct {
	Alert *JobAlert `json:"alert"`
}

func (c *Client) JobAlert(ctx context.Context, s Session) (*JobAlert, error) {
	var out alertEnvelope
	err := c.request(ctx, "GET", "/api/job-alerts", s.AccessToken, "", "application/json", nil, &out, false)
	return out.Alert, err
}

func (c *Client) SaveJobAlert(ctx context.Context, s Session, a JobAlert) (*JobAlert, error) {
	// Turkum nomi ataylab yuborilmaydi — uni backend o'z bazasidan oladi.
	b, _ := json.Marshal(map[string]any{
		"lat": a.Lat, "lng": a.Lng, "radiusM": a.RadiusM,
		"categoryId": a.CategoryID, "placeName": a.PlaceName,
	})
	var out alertEnvelope
	err := c.request(ctx, "PUT", "/api/job-alerts", s.AccessToken, "", "application/json", b, &out, false)
	return out.Alert, err
}

func (c *Client) DeleteJobAlert(ctx context.Context, s Session) error {
	return c.request(ctx, "DELETE", "/api/job-alerts", s.AccessToken, "", "application/json", nil, nil, false)
}
