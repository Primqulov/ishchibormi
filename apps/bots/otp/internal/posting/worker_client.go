package posting

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
)

type WorkerAPI interface {
	Job(context.Context, Session, string) (Job, error)
	Apply(context.Context, Session, string, int) (Application, error)
	Applications(context.Context, Session, string, int, bool) ([]Application, error)
	Application(context.Context, Session, string) (Application, error)
	ApplicationAction(context.Context, Session, string, string, string) (string, error)
}

type Job struct {
	NearbyJob
	OwnerID      string   `json:"ownerId"`
	OwnerName    string   `json:"ownerName"`
	OwnerRating  float64  `json:"ownerRating"`
	ContactPhone string   `json:"contactPhone"`
	LocationText string   `json:"locationText"`
	WorkTimeTo   string   `json:"workTimeTo"`
	Status       string   `json:"status"`
	Images       []string `json:"images"`
}

type Application struct {
	ID                    string `json:"id"`
	ElonID                string `json:"elonId"`
	ElonTitle             string `json:"elonTitle"`
	WorkerID              string `json:"workerId"`
	EmployerID            string `json:"employerId"`
	WorkerName            string `json:"workerName"`
	WorkerPhone           string `json:"workerPhone"`
	OwnerName             string `json:"ownerName"`
	PeopleCount           int    `json:"peopleCount"`
	Amount                int64  `json:"amount"`
	IsNegotiable          bool   `json:"isNegotiable"`
	Status                string `json:"status"`
	CancelReason          string `json:"cancelReason"`
	WorkerConfirmedDone   bool   `json:"workerConfirmedDone"`
	EmployerConfirmedDone bool   `json:"employerConfirmedDone"`
	AppliedAt             string `json:"appliedAt"`
}

func (c *Client) Job(ctx context.Context, s Session, id string) (Job, error) {
	var out Job
	err := c.request(ctx, "GET", "/api/elons/"+url.PathEscape(id), s.AccessToken, "", "application/json", nil, &out, false)
	return out, err
}
func (c *Client) Apply(ctx context.Context, s Session, id string, people int) (Application, error) {
	b, _ := json.Marshal(map[string]int{"peopleCount": people})
	var out Application
	err := c.request(ctx, "POST", "/api/elons/"+url.PathEscape(id)+"/apply", s.AccessToken, "", "application/json", b, &out, false)
	return out, err
}
func (c *Client) Applications(ctx context.Context, s Session, status string, page int, employer bool) ([]Application, error) {
	query := url.Values{"limit": {"5"}, "page": {strconv.Itoa(page)}, "status": {status}}
	path := "/api/my/applications?" + query.Encode()
	if employer {
		var groups map[string][]Application
		err := c.request(ctx, "GET", "/api/my/elons/applications?"+query.Encode(), s.AccessToken, "", "application/json", nil, &groups, false)
		out := []Application{}
		for _, apps := range groups {
			out = append(out, apps...)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].AppliedAt == out[j].AppliedAt {
				return out[i].ID > out[j].ID
			}
			return out[i].AppliedAt > out[j].AppliedAt
		})
		return out, err
	}
	var out []Application
	err := c.request(ctx, "GET", path, s.AccessToken, "", "application/json", nil, &out, false)
	return out, err
}
func (c *Client) Application(ctx context.Context, s Session, id string) (Application, error) {
	var out Application
	err := c.request(ctx, "GET", "/api/applications/"+url.PathEscape(id), s.AccessToken, "", "application/json", nil, &out, false)
	return out, err
}
func (c *Client) ApplicationAction(ctx context.Context, s Session, id, action, reason string) (string, error) {
	b, _ := json.Marshal(map[string]string{"reason": reason})
	var out struct {
		Status string `json:"status"`
	}
	err := c.request(ctx, "POST", "/api/applications/"+url.PathEscape(id)+"/"+action, s.AccessToken, "", "application/json", b, &out, false)
	return out.Status, err
}
