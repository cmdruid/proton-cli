package proton

import "context"

// Session lifecycle, mirroring the "Sessions" page of Proton's account settings
// (packages/components/containers/sessions/) and the calls behind it
// (packages/shared/lib/api/auth.ts: querySessions, revokeSession,
// revokeOtherSessions).

// Session is one authenticated session Proton holds for the account.
type Session struct {
	UID string `json:"uid"`
	// LocalID distinguishes sessions belonging to the same account within one
	// client.
	LocalID int `json:"local_id"`
	// ClientID names the app that created it, e.g. "web-mail" or "ios-mail".
	ClientID string `json:"client_id"`
	// MemberID is set for sessions created by an organisation member.
	MemberID   string `json:"member_id,omitempty"`
	CreateTime int64  `json:"create_time"`
	// Current marks the session this client is using.
	Current bool `json:"current"`
}

// Sessions lists the sessions Proton holds for the account, marking the one this
// client is using.
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var r struct {
		Sessions []struct {
			UID        string
			LocalID    int
			ClientID   string
			MemberID   string
			CreateTime int64
		}
	}
	if err := c.Decode(ctx, Request{Method: "GET", Path: "/auth/v4/sessions"}, &r); err != nil {
		return nil, err
	}
	mine, _, _ := c.Tokens()
	out := make([]Session, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		out = append(out, Session{
			UID: s.UID, LocalID: s.LocalID, ClientID: s.ClientID,
			MemberID: s.MemberID, CreateTime: s.CreateTime,
			Current: s.UID == mine,
		})
	}
	return out, nil
}

// RevokeSession invalidates one session server-side. Revoking the current
// session also makes the sealed key password on disk permanently undecryptable,
// which is the property that makes a leaked session file worthless.
func (c *Client) RevokeSession(ctx context.Context, uid string) error {
	return c.Decode(ctx, Request{Method: "DELETE", Path: "/auth/v4/sessions/" + uid}, nil)
}

// RevokeOtherSessions invalidates every session except this one.
func (c *Client) RevokeOtherSessions(ctx context.Context) error {
	return c.Decode(ctx, Request{Method: "DELETE", Path: "/auth/v4/sessions"}, nil)
}
