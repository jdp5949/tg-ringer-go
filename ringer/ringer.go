// Package ringer rings and messages Telegram users from your own account
// (a userbot, over MTProto) using gotd/td. Placing a private call makes the
// target's phone ring — use it as an urgent alert. No audio is streamed.
package ringer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// Config holds credentials and the session file path.
type Config struct {
	APIID       int
	APIHash     string
	SessionFile string
}

// Client wraps a gotd client for a single connected session.
type Client struct {
	tg  *telegram.Client
	api *tg.Client
}

// Run connects, ensures the session is authorized, and invokes fn.
// Authorization must already exist (run Login first); if not, it errors.
func Run(ctx context.Context, cfg Config, fn func(ctx context.Context, c *Client) error) error {
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: cfg.SessionFile},
	})
	return client.Run(ctx, func(ctx context.Context) error {
		st, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !st.Authorized {
			return fmt.Errorf("session not authorized — run login first")
		}
		return fn(ctx, &Client{tg: client, api: client.API()})
	})
}

// Login runs the interactive auth flow (phone, code, optional 2FA password)
// using the supplied UserAuthenticator and stores the session.
func Login(ctx context.Context, cfg Config, authr auth.UserAuthenticator) error {
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: cfg.SessionFile},
	})
	return client.Run(ctx, func(ctx context.Context) error {
		flow := auth.NewFlow(authr, auth.SendCodeOptions{})
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}
		return nil
	})
}

// resolve turns a +phone number or @username into an input user.
func (c *Client) resolve(ctx context.Context, target string) (tg.InputUserClass, error) {
	if len(target) > 0 && target[0] == '+' {
		res, err := c.api.ContactsImportContacts(ctx, []tg.InputPhoneContact{{
			ClientID:  0,
			Phone:     target,
			FirstName: "alert",
			LastName:  "target",
		}})
		if err != nil {
			return nil, err
		}
		if len(res.Users) == 0 {
			return nil, fmt.Errorf("%s is not on Telegram / not resolvable", target)
		}
		u, ok := res.Users[0].AsNotEmpty()
		if !ok {
			return nil, fmt.Errorf("empty user for %s", target)
		}
		return u.AsInput(), nil
	}

	name := target
	if len(name) > 0 && name[0] == '@' {
		name = name[1:]
	}
	res, err := c.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: name})
	if err != nil {
		return nil, err
	}
	if len(res.Users) == 0 {
		return nil, fmt.Errorf("cannot resolve %s", target)
	}
	u, ok := res.Users[0].AsNotEmpty()
	if !ok {
		return nil, fmt.Errorf("empty user for %s", target)
	}
	return u.AsInput(), nil
}

func randInt() (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<31-1))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

// Ring places a private call so the target's phone rings, waits, then hangs up.
// Returns the call id.
func (c *Client) Ring(ctx context.Context, target string, seconds int) (int64, error) {
	peer, err := c.resolve(ctx, target)
	if err != nil {
		return 0, err
	}

	dhClass, err := c.api.MessagesGetDhConfig(ctx, &tg.MessagesGetDhConfigRequest{
		Version: 0, RandomLength: 256,
	})
	if err != nil {
		return 0, err
	}
	dh, ok := dhClass.(*tg.MessagesDhConfig)
	if !ok {
		return 0, fmt.Errorf("unexpected dh config type %T", dhClass)
	}
	p := new(big.Int).SetBytes(dh.P)
	g := big.NewInt(int64(dh.G))

	aBytes := make([]byte, 256)
	if _, err := rand.Read(aBytes); err != nil {
		return 0, err
	}
	a := new(big.Int).SetBytes(aBytes)
	a.Mod(a, p)
	gA := new(big.Int).Exp(g, a, p)

	// left-pad g_a to 256 bytes before hashing (Telegram expects 256-byte value)
	gABytes := make([]byte, 256)
	gA.FillBytes(gABytes)
	gAHash := sha256.Sum256(gABytes)

	randID, err := randInt()
	if err != nil {
		return 0, err
	}
	res, err := c.api.PhoneRequestCall(ctx, &tg.PhoneRequestCallRequest{
		UserID:   peer,
		RandomID: randID,
		GAHash:   gAHash[:],
		Protocol: tg.PhoneCallProtocol{
			MinLayer:        65,
			MaxLayer:        92,
			UDPP2P:          true,
			UDPReflector:    true,
			LibraryVersions: []string{"4.0.0"},
		},
	})
	if err != nil {
		return 0, err
	}

	var id, accessHash int64
	switch call := res.PhoneCall.(type) {
	case *tg.PhoneCallWaiting:
		id, accessHash = call.ID, call.AccessHash
	case *tg.PhoneCallRequested:
		id, accessHash = call.ID, call.AccessHash
	case *tg.PhoneCall:
		id, accessHash = call.ID, call.AccessHash
	default:
		return 0, fmt.Errorf("unexpected call type %T", res.PhoneCall)
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(seconds) * time.Second):
	}

	_, _ = c.api.PhoneDiscardCall(ctx, &tg.PhoneDiscardCallRequest{
		Peer:     tg.InputPhoneCall{ID: id, AccessHash: accessHash},
		Duration: 0,
		Reason:   &tg.PhoneCallDiscardReasonHangup{},
	})
	return id, nil
}

// Message sends a direct text message to the target. Returns nil on success.
func (c *Client) Message(ctx context.Context, target, text string) error {
	peer, err := c.resolve(ctx, target)
	if err != nil {
		return err
	}
	randID, err := randInt()
	if err != nil {
		return err
	}
	inputPeer := &tg.InputPeerUser{}
	switch u := peer.(type) {
	case *tg.InputUser:
		inputPeer.UserID = u.UserID
		inputPeer.AccessHash = u.AccessHash
	default:
		return fmt.Errorf("unsupported peer type %T", peer)
	}
	_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  text,
		RandomID: int64(randID),
	})
	return err
}

// Self returns the logged-in account.
func (c *Client) Self(ctx context.Context) (*tg.User, error) {
	return c.tg.Self(ctx)
}
