package ses_api

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

// Logging policy for the SES integration.
//
// The debugging added while chasing the SES delivery problems logged at
// Warning level on every send, including a full dump of the domain mapping
// table. On a 50k-recipient campaign that is over a million Warning lines and
// a table scan per recipient, and some of those lines carry recipient
// addresses and subjects.
//
// So verbose tracing is kept -- it is genuinely useful on a live server -- but
// put behind a switch that is off by default and costs one atomic read per
// call when off. Anything that signals a real problem still logs
// unconditionally at Warning or Error.
//
// Enable on a live server by setting the environment variable and restarting
// the core container:
//
//	TEZMAIL_SES_DEBUG=1
//
// The value is read once at first use, so flipping it needs a restart. That is
// deliberate: re-reading per send is exactly the per-message cost this is
// meant to remove.

const sesDebugEnvVar = "TEZMAIL_SES_DEBUG"

var (
	sesDebugOnce sync.Once
	sesDebugOn   bool
)

// sesDebugEnabled reports whether verbose SES tracing is switched on.
func sesDebugEnabled() bool {
	sesDebugOnce.Do(func() {
		v := strings.TrimSpace(strings.ToLower(os.Getenv(sesDebugEnvVar)))
		sesDebugOn = v == "1" || v == "true" || v == "yes" || v == "on"
		if sesDebugOn {
			g.Log().Warningf(context.Background(),
				"[SES] verbose tracing is ON (%s). Expect high log volume on large campaigns; unset it when finished.",
				sesDebugEnvVar)
		}
	})
	return sesDebugOn
}

// sesTracef emits a per-message trace line, but only when verbose tracing is
// enabled. Use for anything that runs once per recipient.
func sesTracef(ctx context.Context, format string, args ...interface{}) {
	if !sesDebugEnabled() {
		return
	}
	g.Log().Debugf(ctx, "[SES-TRACE] "+format, args...)
}

// sesWarnf reports a condition an operator needs to see even when tracing is
// off -- a send rejected, a misconfiguration, a security-relevant input.
// Always emitted, so keep it off the per-successful-message path.
func sesWarnf(ctx context.Context, format string, args ...interface{}) {
	g.Log().Warningf(ctx, "[SES] "+format, args...)
}

// sesErrorf reports a failure that lost or degraded a message. Always emitted.
func sesErrorf(ctx context.Context, format string, args ...interface{}) {
	g.Log().Errorf(ctx, "[SES] "+format, args...)
}
