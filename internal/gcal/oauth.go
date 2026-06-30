package gcal

import (
	"context"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"

	"github.com/Pedro-0101/gix-server/internal/store"
)

var googleCalendarScopes = []string{
	calendar.CalendarEventsScope,
}

type UserToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type Client struct {
	oauthConfig *oauth2.Config
	store       *store.Store
}

func New(clientID, clientSecret, redirectURL string, st *store.Store) *Client {
	return &Client{
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       googleCalendarScopes,
			Endpoint:     google.Endpoint,
		},
		store: st,
	}
}

func (c *Client) AuthURL(state string) string {
	return c.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (c *Client) IsConfigured() bool {
	return c.oauthConfig.ClientID != "" && c.oauthConfig.ClientSecret != ""
}

func (c *Client) Exchange(ctx context.Context, code string) (*UserToken, error) {
	token, err := c.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return &UserToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}, nil
}

func (c *Client) calendarService(ctx context.Context, userID int64) (*calendar.Service, error) {
	token, err := c.store.GetGoogleToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.ExpiresAt,
	}
	httpClient := oauth2.NewClient(ctx, c.oauthConfig.TokenSource(ctx, oauthToken))
	return calendar.New(httpClient)
}

func parseTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
