package prowlarr

import (
	"net/http"

	"github.com/go-resty/resty/v2"
)

func NotFollowMagnet() resty.RedirectPolicy {
	return resty.RedirectPolicyFunc(func(r1 *http.Request, _ []*http.Request) error {
		if r1.URL.Scheme == "magnet" {
			return http.ErrUseLastResponse
		}

		return nil
	})
}
