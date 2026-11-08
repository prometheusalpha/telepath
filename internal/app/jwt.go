package app

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"telepath/internal/jsonpath"
)

type claim struct {
	name, value string
	at          time.Time // set for the claims that are times
}

// jwtClaims reads the payload of a signed JWT, its claims in the order
// they were written. The signature isn't checked: this is only to read.
// ok is false for a token that isn't three dot-separated parts, as
// opaque Bearer tokens are; err is set for one that is but whose payload
// doesn't decode.
func jwtClaims(token string) (claims []claim, ok bool, err error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return nil, false, nil
	}
	// Some issuers keep the padding, which RawURLEncoding refuses.
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, true, err
	}
	v, err := jsonpath.Parse(data)
	if err != nil {
		return nil, true, err
	}
	if v.Kind != jsonpath.Object {
		return nil, true, errNotObject
	}
	for i, k := range v.Keys {
		item := v.Items[i]
		cl := claim{name: k, value: item.String()}
		switch item.Kind {
		case jsonpath.String:
			cl.value = item.Str
		case jsonpath.Number:
			// RFC 7519's NumericDate claims, and OIDC's auth_time.
			if n, err := item.Num.Float64(); err == nil && (k == "exp" || k == "iat" || k == "nbf" || k == "auth_time") {
				cl.at = time.Unix(int64(n), 0)
			}
		}
		claims = append(claims, cl)
	}
	return claims, true, nil
}

var errNotObject = errors.New("the payload isn't a JSON object")

// jwtPayload shows the claims of the token as it is sent, the {{name}}s
// filled; nothing for a token that isn't a JWT.
func (a *app) jwtPayload(c *ui.Context, token string) {
	claims, ok, err := jwtClaims(a.env.Expand(token, nil))
	if !ok {
		return
	}
	t := c.Theme()
	ui.Field(c, "Payload", func() {
		if err != nil {
			ui.Text(c, "Looks like a JWT, but its payload doesn't decode.").FontSize(12).TextColor(t.TextMuted)
			return
		}
		cell := func(s string) ui.Element {
			return ui.Text(c, s).Font(mono).FontFeatures(monoFeatures).FontSize(12).Padding(5, 10)
		}
		ui.Grid(c).ColumnTracks(ui.FitContent(), ui.Fr(1)).MaxWidth(560).Radius(6).Clip().Border(1, t.Border).Children(func() {
			for _, h := range []string{"Claim", "Value"} {
				ui.Text(c, h).FontSize(11).FontWeight(600).TextColor(t.TextMuted).Padding(5, 10).Background(t.Surface)
			}
			now := c.Now()
			for _, cl := range claims {
				cell(cl.name).SingleLine().Selectable()
				ui.Row(c).Gap(2).AlignItems(ui.Center).PaddingEnd(4).Children(func() {
					switch {
					case cl.at.IsZero():
						cell(cl.value).Selectable().Shrink(1).MinWidth(0)
					case cl.name == "exp" && !cl.at.After(now):
						cell(cl.value + "  " + cl.at.Local().Format("2006-01-02 15:04:05") + "  expired").Selectable().Shrink(1).MinWidth(0).TextColor(t.Danger)
					default:
						cell(cl.value + "  " + cl.at.Local().Format("2006-01-02 15:04:05")).Selectable().Shrink(1).MinWidth(0)
					}
					// The value as the token has it, without the date shown.
					if iconButton(c, copyIcon, "Copy claim "+cl.name).Shrink(0).Clicked() {
						c.Services().WriteClipboard(cl.value)
						c.Toast("Copied " + cl.name)
					}
				})
			}
		})
	}).Description("Read from the token, the signature not checked.")
}
